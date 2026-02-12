//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go/modules/mysql"

	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
)

func TestInviteExistingMemberReturnsConflict(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)

	ownerID, _ := users.Create(ctx, "owner@test.com", "owner", "hash")
	memberID, _ := users.Create(ctx, "member@test.com", "member", "hash")

	svc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	teamID, err := svc.CreateTeam(ctx, ownerID, "team-a")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if err := svc.InviteByEmail(ctx, ownerID, teamID, "member@test.com", service.RoleMember); err != nil {
		t.Fatalf("first invite failed: %v", err)
	}
	_ = memberID
	if err := svc.InviteByEmail(ctx, ownerID, teamID, "member@test.com", service.RoleMember); err != service.ErrConflict {
		t.Fatalf("expected conflict on second invite, got: %v", err)
	}
}

func TestCreateTeamCreatesOwnerMembership(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	svc := service.NewTeamService(db, teams, members, users, emailOKSender{})

	ownerID, _ := users.Create(ctx, "owner-create@test.com", "ownercreate", "hash")
	teamID, err := svc.CreateTeam(ctx, ownerID, "team-owner")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	role, ok, err := members.GetRole(ctx, teamID, ownerID)
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if !ok || role != service.RoleOwner {
		t.Fatalf("expected owner membership, ok=%v role=%q", ok, role)
	}

	list, err := svc.ListTeams(ctx, ownerID)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(list) != 1 || list[0].ID != teamID {
		t.Fatalf("unexpected list teams result: %+v", list)
	}
}

func TestCreateTeamRollbackOnAddOwnerError(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	svc := service.NewTeamService(db, teams, members, users, emailOKSender{})

	_, err := svc.CreateTeam(ctx, 999999, "team-bad-owner")
	if err == nil {
		t.Fatalf("expected error for unknown owner user_id")
	}

	var cnt int
	if err := db.GetContext(ctx, &cnt, `SELECT COUNT(*) FROM teams WHERE name = ?`, "team-bad-owner"); err != nil {
		t.Fatalf("count teams: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected rollback, team must not persist, count=%d", cnt)
	}
}

func TestCreateTeamRollbackOnCreateTxError(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	svc := service.NewTeamService(db, teams, members, users, emailOKSender{})

	ownerID, _ := users.Create(ctx, "owner-long@test.com", "ownerlong", "hash")
	longName := strings.Repeat("a", 300)
	_, err := svc.CreateTeam(ctx, ownerID, longName)
	if err == nil {
		t.Fatalf("expected create team error for long name")
	}
}

func TestMemberTaskPatchRules(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)

	ownerID, _ := users.Create(ctx, "owner2@test.com", "owner2", "hash")
	memberID, _ := users.Create(ctx, "member2@test.com", "member2", "hash")
	outsiderID, _ := users.Create(ctx, "outsider@test.com", "outsider", "hash")

	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)

	teamID, err := teamSvc.CreateTeam(ctx, ownerID, "team-b")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := members.Add(ctx, teamID, memberID, service.RoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}

	taskID, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{
		TeamID: teamID,
		Title:  "task-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	mixed := map[string]json.RawMessage{
		"status": json.RawMessage(`"done"`),
		"title":  json.RawMessage(`"hack"`),
	}
	if _, err := taskSvc.UpdateTask(ctx, memberID, taskID, mixed); err != service.ErrForbidden {
		t.Fatalf("expected forbidden on mixed fields, got %v", err)
	}

	invalidAssignee := map[string]json.RawMessage{
		"assignee_id": json.RawMessage(strconv.AppendInt(nil, outsiderID, 10)),
	}
	if _, err := taskSvc.UpdateTask(ctx, memberID, taskID, invalidAssignee); err != service.ErrBadRequest {
		t.Fatalf("expected bad request on outsider assignee, got %v", err)
	}
}

func TestInviteRulesAndErrors(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)

	ownerID, _ := users.Create(ctx, "owner3@test.com", "owner3", "hash")
	adminID, _ := users.Create(ctx, "admin3@test.com", "admin3", "hash")
	randomID, _ := users.Create(ctx, "random3@test.com", "random3", "hash")

	svc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	teamID, err := svc.CreateTeam(ctx, ownerID, "team-c")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := members.Add(ctx, teamID, adminID, service.RoleAdmin); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	if err := svc.InviteByEmail(ctx, ownerID, 99999, "none@test.com", service.RoleMember); err != service.ErrNotFound {
		t.Fatalf("expected team not found, got %v", err)
	}
	if err := svc.InviteByEmail(ctx, randomID, teamID, "x@test.com", service.RoleMember); err != service.ErrForbidden {
		t.Fatalf("expected forbidden for non-member inviter, got %v", err)
	}
	if err := svc.InviteByEmail(ctx, adminID, teamID, "ghost@test.com", service.RoleAdmin); err != service.ErrForbidden {
		t.Fatalf("expected admin cannot invite admin, got %v", err)
	}
	if err := svc.InviteByEmail(ctx, ownerID, teamID, "ghost@test.com", service.RoleMember); err != service.ErrNotFound {
		t.Fatalf("expected invited user not found, got %v", err)
	}
}

func TestTaskAndCommentFlows(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)

	ownerID, _ := users.Create(ctx, "owner4@test.com", "owner4", "hash")
	adminID, _ := users.Create(ctx, "admin4@test.com", "admin4", "hash")
	memberID, _ := users.Create(ctx, "member4@test.com", "member4", "hash")
	member2ID, _ := users.Create(ctx, "member42@test.com", "member42", "hash")
	outsiderID, _ := users.Create(ctx, "outsider4@test.com", "outsider4", "hash")

	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)

	teamID, err := teamSvc.CreateTeam(ctx, ownerID, "team-d")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	_ = members.Add(ctx, teamID, adminID, service.RoleAdmin)
	_ = members.Add(ctx, teamID, memberID, service.RoleMember)
	_ = members.Add(ctx, teamID, member2ID, service.RoleMember)

	due := time.Now().UTC().Add(24 * time.Hour).Truncate(24 * time.Hour)
	taskID, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{
		TeamID:   teamID,
		Title:    "task-flow",
		DueDate:  &due,
		Priority: "",
		Status:   "",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	taskRow, err := tasks.GetByID(ctx, taskID)
	if err != nil || taskRow == nil {
		t.Fatalf("get task row: %v", err)
	}
	if taskRow.Status != "todo" || taskRow.Priority != "medium" {
		t.Fatalf("expected defaults todo/medium, got %s/%s", taskRow.Status, taskRow.Priority)
	}

	if _, _, err := taskSvc.ListTasks(ctx, outsiderID, service.TaskListInput{TeamID: teamID, Limit: 10, Offset: 0}); err != service.ErrForbidden {
		t.Fatalf("expected forbidden list for outsider, got %v", err)
	}
	if _, _, err := taskSvc.ListTasks(ctx, ownerID, service.TaskListInput{TeamID: 99999, Limit: 10, Offset: 0}); err != service.ErrNotFound {
		t.Fatalf("expected not found team in list, got %v", err)
	}

	unknownPatch := map[string]json.RawMessage{"unknown": json.RawMessage(`1`)}
	if _, err := taskSvc.UpdateTask(ctx, ownerID, taskID, unknownPatch); err != service.ErrBadRequest {
		t.Fatalf("expected bad request unknown field, got %v", err)
	}
	if _, err := taskSvc.UpdateTask(ctx, ownerID, taskID, map[string]json.RawMessage{}); err != service.ErrBadRequest {
		t.Fatalf("expected bad request empty patch, got %v", err)
	}

	nullAssigneePatch := map[string]json.RawMessage{
		"status":      json.RawMessage(`"done"`),
		"assignee_id": json.RawMessage(`null`),
	}
	if _, err := taskSvc.UpdateTask(ctx, memberID, taskID, nullAssigneePatch); err != nil {
		t.Fatalf("member should update status/null assignee: %v", err)
	}
	assignMemberPatch := map[string]json.RawMessage{
		"assignee_id": json.RawMessage(strconv.AppendInt(nil, member2ID, 10)),
	}
	if _, err := taskSvc.UpdateTask(ctx, memberID, taskID, assignMemberPatch); err != nil {
		t.Fatalf("member should assign in-team user: %v", err)
	}

	historyRows, total, err := taskSvc.GetTaskHistory(ctx, ownerID, taskID, 20, 0)
	if err != nil {
		t.Fatalf("get task history: %v", err)
	}
	if total < 2 || len(historyRows) < 2 {
		t.Fatalf("expected at least two history rows after patch, total=%d len=%d", total, len(historyRows))
	}

	if _, err := taskSvc.DeleteTask(ctx, memberID, taskID); err != service.ErrForbidden {
		t.Fatalf("member delete must be forbidden, got %v", err)
	}

	commentID, err := taskSvc.CreateComment(ctx, memberID, taskID, "hello")
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if err := taskSvc.UpdateComment(ctx, memberID, commentID, "updated by author"); err != nil {
		t.Fatalf("author update comment failed: %v", err)
	}
	if err := taskSvc.UpdateComment(ctx, outsiderID, commentID, "hack"); err != service.ErrForbidden {
		t.Fatalf("outsider update comment expected forbidden, got %v", err)
	}
	if err := taskSvc.DeleteComment(ctx, adminID, commentID); err != nil {
		t.Fatalf("admin should delete any comment: %v", err)
	}

	if _, err := taskSvc.DeleteTask(ctx, ownerID, taskID); err != nil {
		t.Fatalf("owner delete task failed: %v", err)
	}

	var deletedRows int
	if err := db.GetContext(ctx, &deletedRows, `SELECT COUNT(*) FROM task_history WHERE task_id = ? AND field_name = 'task_deleted'`, taskID); err != nil {
		t.Fatalf("count deleted history rows: %v", err)
	}
	if deletedRows != 1 {
		t.Fatalf("expected exactly one task_deleted row, got %d", deletedRows)
	}

	if _, err := taskSvc.GetTask(ctx, ownerID, taskID); err != service.ErrNotFound {
		t.Fatalf("expected task not found after delete, got %v", err)
	}

	var cnt int
	if err := db.GetContext(ctx, &cnt, `SELECT COUNT(*) FROM task_comments WHERE task_id = ?`, taskID); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected comments cascade delete, got %d", cnt)
	}
}

func TestTaskAndCommentErrorMatrix(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	db := setupMySQLDB(t, ctx)
	defer db.Close()

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := repository.NewTeamMemberRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewTaskCommentRepository(db)
	history := repository.NewTaskHistoryRepository(db)

	ownerID, _ := users.Create(ctx, "owner5@test.com", "owner5", "hash")
	memberID, _ := users.Create(ctx, "member5@test.com", "member5", "hash")
	outsiderID, _ := users.Create(ctx, "outsider5@test.com", "outsider5", "hash")

	teamSvc := service.NewTeamService(db, teams, members, users, emailOKSender{})
	taskSvc := service.NewTaskService(db, tasks, teams, members, comments, history)

	teamID, err := teamSvc.CreateTeam(ctx, ownerID, "team-e")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := members.Add(ctx, teamID, memberID, service.RoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}

	if _, err := taskSvc.CreateTask(ctx, outsiderID, service.CreateTaskInput{TeamID: teamID, Title: "x"}); err != service.ErrForbidden {
		t.Fatalf("expected forbidden create by outsider, got %v", err)
	}
	if _, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{TeamID: 99999, Title: "x"}); err != service.ErrNotFound {
		t.Fatalf("expected not found team on create, got %v", err)
	}
	if _, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{TeamID: teamID, Title: "x", Status: "bad"}); err != service.ErrBadRequest {
		t.Fatalf("expected bad request invalid status, got %v", err)
	}
	if _, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{TeamID: teamID, Title: "x", Priority: "bad"}); err != service.ErrBadRequest {
		t.Fatalf("expected bad request invalid priority, got %v", err)
	}
	if _, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{TeamID: teamID, Title: "x", AssigneeID: &outsiderID}); err != service.ErrBadRequest {
		t.Fatalf("expected bad request outsider assignee, got %v", err)
	}

	taskID, err := taskSvc.CreateTask(ctx, ownerID, service.CreateTaskInput{TeamID: teamID, Title: "task-e"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := taskSvc.GetTask(ctx, ownerID, 999999); err != service.ErrNotFound {
		t.Fatalf("expected not found get task, got %v", err)
	}
	if _, err := taskSvc.GetTask(ctx, outsiderID, taskID); err != service.ErrForbidden {
		t.Fatalf("expected forbidden get task for outsider, got %v", err)
	}

	if _, err := taskSvc.DeleteTask(ctx, ownerID, 999999); err != service.ErrNotFound {
		t.Fatalf("expected not found delete task, got %v", err)
	}

	if _, err := taskSvc.CreateComment(ctx, ownerID, 999999, "x"); err != service.ErrNotFound {
		t.Fatalf("expected not found create comment, got %v", err)
	}
	if _, err := taskSvc.ListComments(ctx, ownerID, 999999); err != service.ErrNotFound {
		t.Fatalf("expected not found list comments, got %v", err)
	}
	if err := taskSvc.UpdateComment(ctx, ownerID, 999999, "x"); err != service.ErrNotFound {
		t.Fatalf("expected not found update comment, got %v", err)
	}
	if err := taskSvc.DeleteComment(ctx, ownerID, 999999); err != service.ErrNotFound {
		t.Fatalf("expected not found delete comment, got %v", err)
	}
}

func setupMySQLDB(t *testing.T, ctx context.Context) *sqlx.DB {
	t.Helper()

	mysqlC, err := mysql.RunContainer(ctx,
		mysql.WithDatabase("mkk_luna_test"),
		mysql.WithUsername("root"),
		mysql.WithPassword("root"),
	)
	if err != nil {
		t.Fatalf("mysql container: %v", err)
	}
	t.Cleanup(func() { _ = mysqlC.Terminate(ctx) })

	host, err := mysqlC.Host(ctx)
	if err != nil {
		t.Fatalf("mysql host: %v", err)
	}
	port, err := mysqlC.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatalf("mysql port: %v", err)
	}

	dsn := "root:root@tcp(" + host + ":" + port.Port() + ")/mkk_luna_test?parseTime=true&multiStatements=true"

	migrationsPath, err := findMigrationsPathForTeamTask()
	if err != nil {
		t.Fatalf("migrations path: %v", err)
	}
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsPath), "mysql://"+dsn)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate.Up: %v", err)
	}
	t.Cleanup(func() {
		_, _ = m.Close()
	})

	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sqlx open: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("sqlx ping: %v", err)
	}
	return db
}

func findMigrationsPathForTeamTask() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(wd, "migrations")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", os.ErrNotExist
}
