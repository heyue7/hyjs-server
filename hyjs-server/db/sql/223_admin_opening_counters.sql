CREATE TABLE IF NOT EXISTS admin_opening_counters (
 occurrence_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL, counter_key VARCHAR(96) NOT NULL,
 value INT NOT NULL, PRIMARY KEY(occurrence_id,player_id,counter_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
