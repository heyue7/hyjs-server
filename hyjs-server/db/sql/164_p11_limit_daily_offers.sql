CREATE TABLE IF NOT EXISTS `p11_limit_daily_offers` (
 `id` VARCHAR(64) NOT NULL,
 `period_id` VARCHAR(64) NOT NULL,
 `day` VARCHAR(8) NOT NULL,
 `start_at` BIGINT NOT NULL,
 `end_at` BIGINT NOT NULL,
 `rules_json` LONGTEXT NOT NULL,
 `period_hash` VARCHAR(64) NOT NULL,
 `source_hash` VARCHAR(64) NOT NULL,
 `captured_at` BIGINT NOT NULL,
 PRIMARY KEY (`id`),
 KEY `idx_period_id` (`period_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
