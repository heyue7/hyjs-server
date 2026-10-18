CREATE TABLE IF NOT EXISTS `p13_union_fight_weekly_policies` (
 `week_start` CHAR(10) NOT NULL, `group` CHAR(1) NOT NULL,
 `enabled` BOOLEAN NOT NULL DEFAULT 0, `version` VARCHAR(64) NOT NULL,
 `source` TEXT NOT NULL, `rewards` TEXT NOT NULL, `published_at` BIGINT NOT NULL,
 PRIMARY KEY (`week_start`,`group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
