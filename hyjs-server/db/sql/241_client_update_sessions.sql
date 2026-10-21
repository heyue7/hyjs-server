CREATE TABLE IF NOT EXISTS client_update_sessions (
  player_id BIGINT NOT NULL,
  session_hash VARCHAR(64),
  build_code BIGINT NOT NULL,
  startup_id VARCHAR(64),
  login_at BIGINT NOT NULL,
  last_seen_at BIGINT NOT NULL,
  disconnected_at BIGINT NOT NULL,
  maintenance_epoch BIGINT NOT NULL,
  connected BOOLEAN NOT NULL,
  PRIMARY KEY (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
