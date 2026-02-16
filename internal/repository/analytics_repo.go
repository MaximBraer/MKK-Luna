package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type TeamDoneStat struct {
	TeamID       int64  `db:"team_id"`
	TeamName     string `db:"team_name"`
	MembersCount int64  `db:"members_count"`
	DoneCount    int64  `db:"done_count"`
}

type TeamTopCreator struct {
	TeamID       int64 `db:"team_id"`
	UserID       int64 `db:"user_id"`
	CreatedCount int64 `db:"created_count"`
	Rank         int   `db:"rn"`
}

type TaskIntegrityIssue struct {
	TaskID     int64 `db:"task_id"`
	TeamID     int64 `db:"team_id"`
	AssigneeID int64 `db:"assignee_id"`
}

type AnalyticsRepository struct {
	db *sqlx.DB
}

func NewAnalyticsRepository(db *sqlx.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

const teamDoneStatsSQL = `
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

func (r *AnalyticsRepository) GetTeamDoneStats(ctx context.Context, userID int64, from, to time.Time) ([]TeamDoneStat, error) {
	var rows []TeamDoneStat
	if err := r.db.SelectContext(ctx, &rows, teamDoneStatsSQL, from, to, userID); err != nil {
		return nil, err
	}
	return rows, nil
}

const topCreatorsSQL = `
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

func (r *AnalyticsRepository) GetTopCreatorsByTeam(ctx context.Context, userID int64, from, to time.Time, limit int) ([]TeamTopCreator, error) {
	var rows []TeamTopCreator
	if err := r.db.SelectContext(ctx, &rows, topCreatorsSQL, from, to, userID, limit); err != nil {
		return nil, err
	}
	return rows, nil
}

const integrityIssuesSQL = `
SELECT t.id AS task_id, t.team_id, t.assignee_id
FROM tasks t
LEFT JOIN team_members tm
  ON tm.team_id = t.team_id
 AND tm.user_id = t.assignee_id
WHERE t.assignee_id IS NOT NULL
  AND tm.user_id IS NULL
`

func (r *AnalyticsRepository) FindTasksWithAssigneeNotMember(ctx context.Context) ([]TaskIntegrityIssue, error) {
	var rows []TaskIntegrityIssue
	if err := r.db.SelectContext(ctx, &rows, integrityIssuesSQL); err != nil {
		return nil, err
	}
	return rows, nil
}
