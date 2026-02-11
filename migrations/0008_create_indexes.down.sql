DROP INDEX idx_sessions_expires_at ON sessions;
DROP INDEX idx_sessions_user_id ON sessions;

DROP INDEX idx_task_comments_user_id ON task_comments;
DROP INDEX idx_task_comments_task_created ON task_comments;

DROP INDEX idx_task_history_changed_by ON task_history;
DROP INDEX idx_task_history_task_created ON task_history;

DROP INDEX idx_tasks_assignee_id ON tasks;
DROP INDEX idx_tasks_created_by ON tasks;
DROP INDEX idx_tasks_team_updated ON tasks;
DROP INDEX idx_tasks_team_status_assignee_updated ON tasks;

DROP INDEX idx_team_members_user_id ON team_members;

DROP INDEX idx_teams_created_by ON teams;
