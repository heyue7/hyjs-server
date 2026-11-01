CREATE TABLE IF NOT EXISTS p8_gather_attempts (
  player_id BIGINT NOT NULL PRIMARY KEY, token VARCHAR(64) NOT NULL,
  scene VARCHAR(64) NOT NULL, pos_id VARCHAR(64) NOT NULL, generation BIGINT NOT NULL,
  started_at BIGINT NOT NULL, ready_at BIGINT NOT NULL, active BOOLEAN NOT NULL,
  reservation_version INT NOT NULL DEFAULT 0, reservation_line INT NOT NULL DEFAULT 0,
  snapshot LONGTEXT NOT NULL, rewards LONGTEXT NOT NULL,
  KEY idx_p8_gather_due (scene, active, ready_at),
  KEY idx_p8_gather_point (scene, pos_id, active, ready_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
