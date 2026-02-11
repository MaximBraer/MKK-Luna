CREATE INDEX idx_teams_created_by ON teams (created_by);

CREATE INDEX idx_team_members_user_id ON team_members (user_id);

CREATE INDEX idx_tasks_team_status_assignee_updated ON tasks (team_id, status, assignee_id, updated_at);
CREATE INDEX idx_tasks_team_updated ON tasks (team_id, updated_at);
CREATE INDEX idx_tasks_created_by ON tasks (created_by);
CREATE INDEX idx_tasks_assignee_id ON tasks (assignee_id);

CREATE INDEX idx_task_history_task_created ON task_history (task_id, created_at);
CREATE INDEX idx_task_history_changed_by ON task_history (changed_by);

CREATE INDEX idx_task_comments_task_created ON task_comments (task_id, created_at);
CREATE INDEX idx_task_comments_user_id ON task_comments (user_id);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);
