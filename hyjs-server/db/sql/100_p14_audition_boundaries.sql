CREATE TABLE IF NOT EXISTS p14_audition_boundaries (
 season_index INT NOT NULL,
 server_id BIGINT NOT NULL,
 register_start BIGINT NOT NULL,
 register_end BIGINT NOT NULL,
 frozen_at BIGINT NOT NULL,
 count BIGINT NOT NULL,
 calendar LONGBLOB NOT NULL,
 calendar_hash CHAR(64) NOT NULL,
 roster_hash CHAR(64) NOT NULL,
 source TEXT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,server_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
