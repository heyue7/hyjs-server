CREATE TABLE IF NOT EXISTS p11_arena_daily_awards (
 day VARCHAR(8) NOT NULL,
 player_id BIGINT NOT NULL,
 position BIGINT NOT NULL,
 rewards TEXT NOT NULL,
 snapshot_hash VARCHAR(64) NOT NULL,
 rank_proof VARCHAR(64) NOT NULL,
 delivery_id BIGINT NOT NULL,
 source_key VARBINARY(191) NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(day,player_id),
 UNIQUE KEY idx_p11_arena_daily_award_source(source_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
