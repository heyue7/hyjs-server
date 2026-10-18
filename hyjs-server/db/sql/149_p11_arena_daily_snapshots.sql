CREATE TABLE IF NOT EXISTS p11_arena_daily_snapshots (
 day VARCHAR(8) NOT NULL,
 start_at BIGINT NOT NULL,
 end_at BIGINT NOT NULL,
 offer_hash VARCHAR(64) NOT NULL,
 status VARCHAR(32) NOT NULL,
 reason TEXT NOT NULL,
 ranks_json LONGTEXT NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 captured_at BIGINT NOT NULL,
 PRIMARY KEY(day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
