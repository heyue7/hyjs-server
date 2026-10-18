CREATE TABLE IF NOT EXISTS `p13_union_fight_transport_stats` (
  `match_id` CHAR(64) NOT NULL,
  `player_id` BIGINT NOT NULL,
  `guild_id` VARCHAR(64) NOT NULL,
  `ore` BIGINT NOT NULL,
  `deposits` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  PRIMARY KEY (`match_id`,`player_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
