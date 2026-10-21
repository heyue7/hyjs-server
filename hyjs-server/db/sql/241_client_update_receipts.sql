CREATE TABLE IF NOT EXISTS client_update_receipts (
  batch_id BIGINT NOT NULL,
  startup_id VARCHAR(64) NOT NULL,
  kind VARCHAR(24) NOT NULL,
  player_id BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (batch_id, startup_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
