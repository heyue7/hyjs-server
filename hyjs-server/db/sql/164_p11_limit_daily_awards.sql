CREATE TABLE IF NOT EXISTS `p11_limit_daily_awards` (
 `offer_id` VARCHAR(64) NOT NULL,
 `player_id` BIGINT NOT NULL,
 `period_id` VARCHAR(64) NOT NULL,
 `day` VARCHAR(8) NOT NULL,
 `rank` BIGINT NOT NULL,
 `bonus` BIGINT NOT NULL,
 `before_score` BIGINT NOT NULL,
 `after_score` BIGINT NOT NULL,
 `snapshot_hash` VARCHAR(64) NOT NULL,
 `source_key` VARCHAR(191) NOT NULL,
 `source_hash` VARCHAR(64) NOT NULL,
 `created_at` BIGINT NOT NULL,
 PRIMARY KEY (`offer_id`,`player_id`),
 KEY `idx_period_id` (`period_id`),
 UNIQUE KEY `uq_source_key` (`source_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
