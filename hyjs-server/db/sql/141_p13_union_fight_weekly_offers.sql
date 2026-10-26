CREATE TABLE IF NOT EXISTS `p13_union_fight_weekly_offers` (
 `week_start` CHAR(10) NOT NULL, `group` CHAR(1) NOT NULL,
 `rules` LONGBLOB NOT NULL, `rules_hash` CHAR(64) NOT NULL,
 `first_bye_id` CHAR(64) NOT NULL DEFAULT '',
 `frozen_at` BIGINT NOT NULL, `first_match_id` CHAR(64) NOT NULL,
 PRIMARY KEY (`week_start`,`group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
