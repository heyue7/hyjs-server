CREATE TABLE IF NOT EXISTS p14_audition_qualifications (
  `recovery_source` varchar(1024) NOT NULL DEFAULT '',
  `recovery_published_at` bigint NOT NULL DEFAULT 0,
 season_index INT NOT NULL,
 server_id BIGINT NOT NULL,
 boundary_hash CHAR(64) NOT NULL,
 round_count BIGINT NOT NULL,
 players LONGBLOB NOT NULL,
 results_hash CHAR(64) NOT NULL,
 qualified_at BIGINT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,server_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
