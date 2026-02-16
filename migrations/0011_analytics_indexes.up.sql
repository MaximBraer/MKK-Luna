CREATE INDEX idx_tasks_created_at_team_created_by ON tasks (created_at, team_id, created_by);
CREATE INDEX idx_tasks_team_assignee ON tasks (team_id, assignee_id);
