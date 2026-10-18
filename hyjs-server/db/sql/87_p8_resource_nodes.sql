CREATE TABLE IF NOT EXISTS p8_resource_nodes (
  player_id BIGINT NOT NULL, scene VARCHAR(64) NOT NULL, pos_id VARCHAR(64) NOT NULL,
  resource_id VARCHAR(64) NOT NULL, remaining INT NOT NULL, refresh_at BIGINT NOT NULL,
  generation BIGINT NOT NULL, snapshot LONGTEXT NOT NULL,
  PRIMARY KEY (player_id, scene, pos_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
