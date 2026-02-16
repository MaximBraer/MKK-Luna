CREATE TABLE team_members (
  team_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role ENUM('owner','admin','member') NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (team_id, user_id),
  CONSTRAINT fk_team_members_team_id FOREIGN KEY (team_id)
    REFERENCES teams(id) ON DELETE CASCADE,
  CONSTRAINT fk_team_members_user_id FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
