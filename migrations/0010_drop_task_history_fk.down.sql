ALTER TABLE task_history
  ADD CONSTRAINT fk_task_history_task_id FOREIGN KEY (task_id)
  REFERENCES tasks(id) ON DELETE CASCADE;
