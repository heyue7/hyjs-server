CREATE TABLE IF NOT EXISTS p14_audition_match_policies (
  `auto_recovery_enabled` tinyint(1) NOT NULL DEFAULT 0,
  `auto_recovery_published_at` bigint NOT NULL DEFAULT 0,
  `auto_recovery_source` varchar(1024) NOT NULL DEFAULT '',
 id BIGINT NOT NULL PRIMARY KEY,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
