CREATE TABLE IF NOT EXISTS p12_world_boss_boosts (
  instance_id VARCHAR(64) NOT NULL,
  player_id BIGINT NOT NULL,
  property_id VARCHAR(64) NOT NULL,
  property VARCHAR(128) NOT NULL,
  kind BIGINT NOT NULL,
  paid BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (instance_id, player_id, property_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
