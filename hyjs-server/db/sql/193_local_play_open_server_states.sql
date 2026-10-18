CREATE TABLE IF NOT EXISTS local_play_open_server_states (
 cycle_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL, task_id INT NOT NULL, achieved BOOLEAN NOT NULL, progress INT NOT NULL, claims INT NOT NULL, used_stock INT NOT NULL, PRIMARY KEY(cycle_id,task_id), KEY idx_local_play_os_state_owner(player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
