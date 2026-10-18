CREATE TABLE IF NOT EXISTS `p13_union_fight_settlement_intents` (
  `match_id` CHAR(64) NOT NULL,
  `player_id` BIGINT NOT NULL,
  `target_id` BIGINT NOT NULL,
  `stamp` BIGINT NOT NULL,
  `observed_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`match_id`,`player_id`,`target_id`,`stamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
