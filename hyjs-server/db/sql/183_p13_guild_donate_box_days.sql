CREATE TABLE IF NOT EXISTS `p13_guild_donate_box_days` (
 `guild_id` varchar(64) NOT NULL,
 `day` varchar(10) NOT NULL,
 `version` varchar(128) NOT NULL,
 `source` varchar(1024) NOT NULL,
 `rules_json` text NOT NULL,
 `rules_hash` char(64) NOT NULL,
 `published_at` bigint NOT NULL,
 `created_at` bigint NOT NULL,
 `score` int NOT NULL,
 PRIMARY KEY (`guild_id`,`day`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
