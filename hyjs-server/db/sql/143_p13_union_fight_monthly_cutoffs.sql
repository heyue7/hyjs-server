CREATE TABLE IF NOT EXISTS `p13_union_fight_monthly_cutoffs` (
 `month` CHAR(7) NOT NULL, `offer_hash` CHAR(64) NOT NULL,
 `cutoff_at` BIGINT NOT NULL, `captured_at` BIGINT NOT NULL,
 `members` LONGBLOB NOT NULL, `guilds` LONGBLOB NOT NULL,
 `snapshot_hash` CHAR(64) NOT NULL, `source` TEXT NOT NULL, PRIMARY KEY (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
