//go:build integration

package integration

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/repository"
)

const explainTeamDoneStatsSQL = `
SELECT
  t.id AS team_id,
  t.name AS team_name,
  COALESCE(m.members_count, 0) AS members_count,
  COALESCE(d.done_count, 0) AS done_count
FROM teams t
LEFT JOIN (
    SELECT team_id, COUNT(*) AS members_count
    FROM team_members
    GROUP BY team_id
) m ON m.team_id = t.id
LEFT JOIN (
    SELECT team_id, COUNT(*) AS done_count
    FROM tasks
    WHERE status='done'
      AND updated_at >= ?
      AND updated_at < ?
    GROUP BY team_id
) d ON d.team_id = t.id
WHERE t.id IN (
  SELECT team_id
  FROM team_members
  WHERE user_id = ?
    AND role IN ('owner','admin')
)
`

const explainTopCreatorsSQL = `
SELECT *
FROM (
  SELECT
    team_id,
    user_id,
    created_count,
    ROW_NUMBER() OVER (
      PARTITION BY team_id
      ORDER BY created_count DESC, user_id ASC
    ) AS rn
  FROM (
      SELECT
        team_id,
        created_by AS user_id,
        COUNT(*) AS created_count
      FROM tasks
      WHERE created_at >= ?
        AND created_at < ?
        AND team_id IN (
          SELECT team_id
          FROM team_members
          WHERE user_id = ?
            AND role IN ('owner','admin')
        )
      GROUP BY team_id, created_by
  ) base
) ranked
WHERE rn <= ?
`

const explainIntegritySQL = `
SELECT t.id AS task_id, t.team_id, t.assignee_id
FROM tasks t
LEFT JOIN team_members tm
  ON tm.team_id = t.team_id
 AND tm.user_id = t.assignee_id
WHERE t.assignee_id IS NOT NULL
  AND tm.user_id IS NULL
`

func TestAnalyticsDoneStatsCounts(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	userRepo := repository.NewUserRepository(db)
	u1, _ := userRepo.Create(ctx, "a@test.com", "a", "hash")
	u2, _ := userRepo.Create(ctx, "b@test.com", "b", "hash")
	u3, _ := userRepo.Create(ctx, "c@test.com", "c", "hash")

	teamID1 := insertTeam(t, ctx, db, "team-1", u1)
	teamID2 := insertTeam(t, ctx, db, "team-2", u3)

	memberRepo := repository.NewTeamMemberRepository(db)
	_ = memberRepo.Add(ctx, teamID1, u1, "owner")
	_ = memberRepo.Add(ctx, teamID1, u2, "member")
	_ = memberRepo.Add(ctx, teamID2, u3, "owner")

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	inRange := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	outRange := time.Date(2025, 12, 25, 12, 0, 0, 0, time.UTC)

	insertTaskWithTimes(t, ctx, db, teamID1, "done-1", "done", "medium", inRange, inRange, nil)
	insertTaskWithTimes(t, ctx, db, teamID1, "done-2", "done", "medium", inRange, inRange, nil)
	insertTaskWithTimes(t, ctx, db, teamID1, "done-old", "done", "medium", outRange, outRange, nil)
	insertTaskWithTimes(t, ctx, db, teamID1, "todo", "todo", "medium", inRange, inRange, nil)
	insertTaskWithTimes(t, ctx, db, teamID2, "done-team2", "done", "medium", inRange, inRange, nil)

	analytics := repository.NewAnalyticsRepository(db)
	stats, err := analytics.GetTeamDoneStats(ctx, u1, from, to)
	if err != nil {
		t.Fatalf("GetTeamDoneStats: %v", err)
	}

	got := make(map[int64]repository.TeamDoneStat, len(stats))
	for _, s := range stats {
		got[s.TeamID] = s
	}

	if s := got[teamID1]; s.MembersCount != 2 || s.DoneCount != 2 {
		t.Fatalf("team1 expected members=2 done=2, got members=%d done=%d", s.MembersCount, s.DoneCount)
	}
	if _, ok := got[teamID2]; ok {
		t.Fatalf("team2 should not be visible for user %d", u1)
	}
}

func TestAnalyticsDoneStatsEmptyWindow(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	userRepo := repository.NewUserRepository(db)
	u1, _ := userRepo.Create(ctx, "dw1@test.com", "dw1", "hash")
	u2, _ := userRepo.Create(ctx, "dw2@test.com", "dw2", "hash")

	teamID := insertTeam(t, ctx, db, "team-empty", u1)
	memberRepo := repository.NewTeamMemberRepository(db)
	_ = memberRepo.Add(ctx, teamID, u1, "owner")
	_ = memberRepo.Add(ctx, teamID, u2, "member")

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	insertTaskWithTimes(t, ctx, db, teamID, "done-out", "done", "medium", now, now, &u1)

	analytics := repository.NewAnalyticsRepository(db)
	stats, err := analytics.GetTeamDoneStats(ctx, u1, from, to)
	if err != nil {
		t.Fatalf("GetTeamDoneStats: %v", err)
	}

	var found repository.TeamDoneStat
	for _, s := range stats {
		if s.TeamID == teamID {
			found = s
			break
		}
	}
	if found.TeamID != teamID {
		t.Fatalf("team not found in stats")
	}
	if found.MembersCount != 2 || found.DoneCount != 0 {
		t.Fatalf("expected members=2 done=0, got members=%d done=%d", found.MembersCount, found.DoneCount)
	}
}

func TestAnalyticsTopCreatorsRanking(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	userRepo := repository.NewUserRepository(db)
	u1, _ := userRepo.Create(ctx, "u1@test.com", "u1", "hash")
	u2, _ := userRepo.Create(ctx, "u2@test.com", "u2", "hash")
	u3, _ := userRepo.Create(ctx, "u3@test.com", "u3", "hash")

	teamID1 := insertTeam(t, ctx, db, "team-a", u1)
	teamID2 := insertTeam(t, ctx, db, "team-b", u2)

	memberRepo := repository.NewTeamMemberRepository(db)
	_ = memberRepo.Add(ctx, teamID1, u1, "owner")
	_ = memberRepo.Add(ctx, teamID2, u2, "owner")

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	inRange := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	insertTaskWithTimes(t, ctx, db, teamID1, "t1-u1-1", "todo", "medium", inRange, inRange, &u1)
	insertTaskWithTimes(t, ctx, db, teamID1, "t1-u1-2", "todo", "medium", inRange, inRange, &u1)
	insertTaskWithTimes(t, ctx, db, teamID1, "t1-u2-1", "todo", "medium", inRange, inRange, &u2)
	insertTaskWithTimes(t, ctx, db, teamID1, "t1-u2-2", "todo", "medium", inRange, inRange, &u2)
	insertTaskWithTimes(t, ctx, db, teamID1, "t1-u3-1", "todo", "medium", inRange, inRange, &u3)

	insertTaskWithTimes(t, ctx, db, teamID2, "t2-u1-1", "todo", "medium", inRange, inRange, &u1)
	insertTaskWithTimes(t, ctx, db, teamID2, "t2-u1-2", "todo", "medium", inRange, inRange, &u1)
	insertTaskWithTimes(t, ctx, db, teamID2, "t2-u1-3", "todo", "medium", inRange, inRange, &u1)
	insertTaskWithTimes(t, ctx, db, teamID2, "t2-u2-1", "todo", "medium", inRange, inRange, &u2)

	analytics := repository.NewAnalyticsRepository(db)
	rows, err := analytics.GetTopCreatorsByTeam(ctx, u1, from, to, 3)
	if err != nil {
		t.Fatalf("GetTopCreatorsByTeam: %v", err)
	}

	byTeam := map[int64][]repository.TeamTopCreator{}
	for _, r := range rows {
		byTeam[r.TeamID] = append(byTeam[r.TeamID], r)
	}
	for _, list := range byTeam {
		sort.Slice(list, func(i, j int) bool { return list[i].Rank < list[j].Rank })
	}

	gotTeam1 := byTeam[teamID1]
	if len(gotTeam1) != 3 {
		t.Fatalf("team1 expected 3 rows, got %d", len(gotTeam1))
	}
	if gotTeam1[0].UserID != minInt64(u1, u2) || gotTeam1[0].CreatedCount != 2 {
		t.Fatalf("team1 rank1 mismatch: %+v", gotTeam1[0])
	}
	if gotTeam1[1].CreatedCount != 2 || gotTeam1[1].UserID == gotTeam1[0].UserID {
		t.Fatalf("team1 rank2 mismatch: %+v", gotTeam1[1])
	}
	if gotTeam1[2].UserID != u3 || gotTeam1[2].CreatedCount != 1 {
		t.Fatalf("team1 rank3 mismatch: %+v", gotTeam1[2])
	}

	gotTeam2 := byTeam[teamID2]
	if len(gotTeam2) != 0 {
		t.Fatalf("team2 should not be visible for user %d, got %d rows", u1, len(gotTeam2))
	}
}

func TestAnalyticsIntegrityQuery(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	userRepo := repository.NewUserRepository(db)
	u1, _ := userRepo.Create(ctx, "i1@test.com", "i1", "hash")
	u2, _ := userRepo.Create(ctx, "i2@test.com", "i2", "hash")

	teamID := insertTeam(t, ctx, db, "team-x", u1)
	memberRepo := repository.NewTeamMemberRepository(db)
	_ = memberRepo.Add(ctx, teamID, u1, "owner")

	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	taskIDGood := insertTaskWithTimes(t, ctx, db, teamID, "good", "todo", "medium", now, now, &u1)
	taskIDBad := insertTaskWithTimes(t, ctx, db, teamID, "bad", "todo", "medium", now, now, &u2)

	if _, err := db.ExecContext(ctx, `UPDATE tasks SET assignee_id = ? WHERE id = ?`, u1, taskIDGood); err != nil {
		t.Fatalf("set assignee good: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET assignee_id = ? WHERE id = ?`, u2, taskIDBad); err != nil {
		t.Fatalf("set assignee bad: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET assignee_id = NULL WHERE id = ?`, taskIDGood); err != nil {
		t.Fatalf("set assignee good null: %v", err)
	}

	analytics := repository.NewAnalyticsRepository(db)
	rows, err := analytics.FindTasksWithAssigneeNotMember(ctx)
	if err != nil {
		t.Fatalf("FindTasksWithAssigneeNotMember: %v", err)
	}

	if len(rows) != 1 || rows[0].TaskID != taskIDBad {
		t.Fatalf("expected one integrity issue for task %d, got %+v", taskIDBad, rows)
	}
}

func TestAnalyticsExplainUsesIndexes(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	userRepo := repository.NewUserRepository(db)
	u1, _ := userRepo.Create(ctx, "e1@test.com", "e1", "hash")
	teamID := insertTeam(t, ctx, db, "team-explain", u1)
	memberRepo := repository.NewTeamMemberRepository(db)
	_ = memberRepo.Add(ctx, teamID, u1, "owner")

	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	insertTaskWithTimes(t, ctx, db, teamID, "ex", "done", "medium", now, now, &u1)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	checkExplain(t, db, explainTeamDoneStatsSQL, []any{from, to, u1}, map[string]bool{"teams": true, "team_members": true, "tasks": true})
	checkExplain(t, db, explainTopCreatorsSQL, []any{from, to, u1, 3}, map[string]bool{"tasks": true})
	checkExplain(t, db, explainIntegritySQL, nil, map[string]bool{"tasks": true, "team_members": true})
}

type explainRow struct {
	Table string         `db:"table"`
	Type  string         `db:"type"`
	Key   sql.NullString `db:"key"`
}

func checkExplain(t *testing.T, db *sqlx.DB, query string, args []any, tables map[string]bool) {
	t.Helper()

	explainSQL := "EXPLAIN " + query
	var rows []explainRow
	if err := db.Unsafe().Select(&rows, explainSQL, args...); err != nil {
		t.Fatalf("explain query failed: %v", err)
	}
	for _, r := range rows {
		if !tables[r.Table] {
			continue
		}
		if r.Type == "ALL" {
			t.Fatalf("explain type ALL for table %s", r.Table)
		}
		if !r.Key.Valid || r.Key.String == "" {
			t.Fatalf("explain key is NULL for table %s", r.Table)
		}
	}
}

func insertTeam(t *testing.T, ctx context.Context, db *sqlx.DB, name string, createdBy int64) int64 {
	t.Helper()
	res, err := db.ExecContext(ctx, `INSERT INTO teams (name, created_by) VALUES (?, ?)`, name, createdBy)
	if err != nil {
		t.Fatalf("insert team: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("team last insert id: %v", err)
	}
	return id
}

func insertTaskWithTimes(t *testing.T, ctx context.Context, db *sqlx.DB, teamID int64, title, status, priority string, createdAt, updatedAt time.Time, createdBy *int64) int64 {
	t.Helper()

	var createdByVal any
	if createdBy != nil {
		createdByVal = *createdBy
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO tasks (team_id, title, status, priority, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, teamID, title, status, priority, createdByVal, createdAt, updatedAt)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("task last insert id: %v", err)
	}
	return id
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
