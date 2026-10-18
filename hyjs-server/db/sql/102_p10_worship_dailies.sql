CREATE TABLE IF NOT EXISTS p10_worship_dailies (
 player_id BIGINT NOT NULL,
 day VARCHAR(10) NOT NULL,
 limit_count BIGINT NOT NULL,
 reward TEXT NOT NULL,
 configuration_hash CHAR(64) NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(player_id,day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
