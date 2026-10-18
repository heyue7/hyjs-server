CREATE TABLE IF NOT EXISTS server_game_clock_recoveries (
 server_id BIGINT NOT NULL,
 player_id BIGINT NOT NULL,
 high_days BIGINT NOT NULL DEFAULT 0,
 updated_at_millis BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(server_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
