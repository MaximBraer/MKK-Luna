CREATE TABLE task_history (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  changed_by BIGINT NULL,
  field_name VARCHAR(64) NOT NULL,
  old_value JSON NULL,
  new_value JSON NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  CONSTRAINT fk_task_history_task_id FOREIGN KEY (task_id)
    REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_task_history_changed_by FOREIGN KEY (changed_by)
    REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
