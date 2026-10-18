CREATE TABLE IF NOT EXISTS `p11_limit_fight_policies` (
 `id` BIGINT NOT NULL,
 `enabled` TINYINT(1) NOT NULL,
 `version` VARCHAR(64) NOT NULL,
 `source` TEXT NOT NULL,
 `published_at` BIGINT NOT NULL,
 `start_at` BIGINT NOT NULL,
 `end_at` BIGINT NOT NULL,
 `match_millis` BIGINT NOT NULL,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
