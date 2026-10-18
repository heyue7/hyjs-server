CREATE TABLE IF NOT EXISTS `p13_union_fight_guild_mission_facts` (
 `match_id` CHAR(64) NOT NULL, `guild_id` VARCHAR(64) NOT NULL,
 `factory_guid` BIGINT NOT NULL, `player_id` BIGINT NOT NULL,
 `prestige` INT NOT NULL, `offer_hash` CHAR(64) NOT NULL,
 `occurred_at` BIGINT NOT NULL, `source` TEXT NOT NULL,
 PRIMARY KEY (`match_id`,`guild_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
