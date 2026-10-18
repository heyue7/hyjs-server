CREATE TABLE IF NOT EXISTS local_play_activity_bindings (
 player_id BIGINT NOT NULL, activity_id VARCHAR(64) NOT NULL, cycle_id VARCHAR(64) NOT NULL, revision BIGINT NOT NULL, PRIMARY KEY(player_id,activity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
