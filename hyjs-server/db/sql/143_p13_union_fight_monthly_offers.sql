CREATE TABLE IF NOT EXISTS `p13_union_fight_monthly_offers` (
 `month` CHAR(7) NOT NULL, `start_at` BIGINT NOT NULL, `cutoff_at` BIGINT NOT NULL,
 `rules` LONGBLOB NOT NULL, `rules_hash` CHAR(64) NOT NULL, `frozen_at` BIGINT NOT NULL,
 PRIMARY KEY (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
