CREATE TABLE IF NOT EXISTS `p13_guild_donate_box_sources` (
 `player_id` bigint NOT NULL,
 `stamp` bigint NOT NULL,
 `guild_id` varchar(64) NOT NULL,
 `day` varchar(10) NOT NULL,
 `joined_at` bigint NOT NULL,
 `donate_type` varchar(16) NOT NULL,
 `amount` int NOT NULL,
 `score` int NOT NULL,
 `created_at` bigint NOT NULL,
 `source_hash` char(64) NOT NULL,
 PRIMARY KEY (`player_id`,`stamp`),
 KEY idx_p13_box_source_day (`guild_id`,`day`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
