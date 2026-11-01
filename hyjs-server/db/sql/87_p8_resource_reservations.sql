CREATE TABLE IF NOT EXISTS p8_resource_reservations (
  server_id BIGINT NOT NULL, scene VARCHAR(64) NOT NULL, pos_id VARCHAR(64) NOT NULL,
  line INT NOT NULL, player_id BIGINT NOT NULL DEFAULT 0,
  token VARCHAR(64) NOT NULL DEFAULT '', occupied_until BIGINT NOT NULL DEFAULT 0,
  replay_guard BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (server_id, scene, pos_id, line)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
