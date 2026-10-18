CREATE TABLE IF NOT EXISTS `p13_union_fight_monthly_titles` (
 `month` CHAR(7) NOT NULL, `player_id` BIGINT NOT NULL, `title_id` VARCHAR(32) NOT NULL,
 `guild_id` VARCHAR(64) NOT NULL, `rank` INT NOT NULL, `role` INT NOT NULL,
 `granted_at` BIGINT NOT NULL, `expires_at` BIGINT NOT NULL,
 `effects` VARCHAR(1024) NOT NULL, `result_hash` CHAR(64) NOT NULL,
 `source` TEXT NOT NULL, PRIMARY KEY (`month`,`player_id`,`title_id`),
 KEY `idx_p13_union_fight_monthly_titles_player_id` (`player_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
