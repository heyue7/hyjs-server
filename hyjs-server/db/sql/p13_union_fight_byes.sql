CREATE TABLE IF NOT EXISTS `p13_union_fight_byes` (
 `id` CHAR(64) NOT NULL, `guild_id` VARCHAR(64) NOT NULL, `day` CHAR(10) NOT NULL,
 `round` INT NOT NULL, `group` CHAR(1) NOT NULL,
 `start_at` BIGINT NOT NULL, `end_at` BIGINT NOT NULL, `created_at` BIGINT NOT NULL,
 `win_prestige` INT NOT NULL, `snapshot` LONGBLOB NOT NULL,
 `snapshot_hash` CHAR(64) NOT NULL, `source_hash` CHAR(64) NOT NULL, `source` TEXT NOT NULL,
 PRIMARY KEY (`id`), UNIQUE KEY `p13_uf_bye_round` (`guild_id`,`day`,`round`),
 KEY `p13_uf_bye_period` (`day`,`group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
