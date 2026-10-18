CREATE TABLE IF NOT EXISTS `p13_union_fight_monthly_policies` (
 `month` CHAR(7) NOT NULL, `enabled` BOOLEAN NOT NULL DEFAULT FALSE,
 `version` VARCHAR(64) NOT NULL, `source` TEXT NOT NULL, `published_at` BIGINT NOT NULL,
 `rewards` TEXT NOT NULL, `member_title` BOOLEAN NOT NULL DEFAULT FALSE,
 `chairman_title` BOOLEAN NOT NULL DEFAULT FALSE, PRIMARY KEY (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
