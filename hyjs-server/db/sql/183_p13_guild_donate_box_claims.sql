CREATE TABLE IF NOT EXISTS `p13_guild_donate_box_claims` (
 `player_id` bigint NOT NULL,
 `day` varchar(10) NOT NULL,
 `ordinal` bigint NOT NULL,
 `guild_id` varchar(64) NOT NULL,
 `joined_at` bigint NOT NULL,
 `box_id` char(64) NOT NULL,
 `stamp` bigint NOT NULL,
 `score` int NOT NULL,
 `threshold` int NOT NULL,
 `reward` varchar(128) NOT NULL,
 `version` varchar(128) NOT NULL,
 `created_at` bigint NOT NULL,
 `response` longblob NOT NULL,
 `response_hash` char(64) NOT NULL,
 PRIMARY KEY (`player_id`,`day`,`ordinal`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
