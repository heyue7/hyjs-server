CREATE TABLE IF NOT EXISTS `p13_guild_tech_upgrades` (
 `guild_id` varchar(64) NOT NULL, `tech_type` varchar(16) NOT NULL, `level` bigint NOT NULL, `building` varchar(32) NOT NULL,
 `player_id` bigint NOT NULL, `stamp` bigint NOT NULL, `applied_at` bigint NOT NULL, `coin` bigint NOT NULL, `badge` int NOT NULL,
 `prop` varchar(128) NOT NULL, `offer_hash` char(64) NOT NULL, `original_row_hash` char(64) NOT NULL, `source_hash` char(64) NOT NULL,
 PRIMARY KEY (`guild_id`,`tech_type`,`level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
