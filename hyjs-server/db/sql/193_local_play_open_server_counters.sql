CREATE TABLE IF NOT EXISTS local_play_open_server_counters (
 cycle_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL, counter_key VARCHAR(64) NOT NULL, value INT NOT NULL, PRIMARY KEY(cycle_id,counter_key), KEY idx_local_play_os_counter_owner(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
