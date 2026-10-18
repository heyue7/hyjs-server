CREATE TABLE IF NOT EXISTS `p11_limit_fight_offers` (
 `version` VARCHAR(64) NOT NULL,
 `snapshot` LONGTEXT NOT NULL,
 `source_hash` VARCHAR(64) NOT NULL,
 `captured_at` BIGINT NOT NULL,
 PRIMARY KEY (`version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
