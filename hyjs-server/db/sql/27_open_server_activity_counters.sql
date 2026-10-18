CREATE TABLE IF NOT EXISTS open_server_activity_counters (
 player_id BIGINT NOT NULL, counter_key VARCHAR(64) NOT NULL, value INT NOT NULL, PRIMARY KEY(player_id, counter_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
