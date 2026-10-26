CREATE TABLE IF NOT EXISTS `p13_union_fight_weekly_results` (
 `week_start` CHAR(10) NOT NULL, `group` CHAR(1) NOT NULL,
 `offer_hash` CHAR(64) NOT NULL, `rankings` LONGBLOB NOT NULL,
 `awards` LONGBLOB NOT NULL, `result_hash` CHAR(64) NOT NULL,
 `settled_at` BIGINT NOT NULL, `delivered_at` BIGINT NOT NULL DEFAULT 0,
 `source` TEXT NOT NULL, `bye_directory` LONGBLOB NULL,
 PRIMARY KEY (`week_start`,`group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
