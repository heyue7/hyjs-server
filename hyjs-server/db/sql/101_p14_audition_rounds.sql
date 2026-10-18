CREATE TABLE IF NOT EXISTS p14_audition_rounds (
 season_index INT NOT NULL,
 server_id BIGINT NOT NULL,
 round_index BIGINT NOT NULL,
 boundary_hash CHAR(64) NOT NULL,
 participants LONGBLOB NOT NULL,
 match_hash CHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,server_id,round_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
