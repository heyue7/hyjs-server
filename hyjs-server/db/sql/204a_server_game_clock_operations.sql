CREATE TABLE IF NOT EXISTS server_game_clock_operations (
 server_id BIGINT NOT NULL,
 request_id VARCHAR(36) NOT NULL,
 actor_id BIGINT NOT NULL,
 delta_days BIGINT NOT NULL,
 response LONGBLOB NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(server_id,request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
