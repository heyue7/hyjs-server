CREATE TABLE IF NOT EXISTS p12_world_boss_awards (
  instance_id VARCHAR(64) NOT NULL,
  player_id BIGINT NOT NULL,
  kind BIGINT NOT NULL,
  policy_name VARCHAR(96) NOT NULL,
  reward TEXT NOT NULL,
  delivery_id BIGINT NOT NULL,
  expires_at BIGINT NOT NULL,
  PRIMARY KEY (instance_id, player_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
