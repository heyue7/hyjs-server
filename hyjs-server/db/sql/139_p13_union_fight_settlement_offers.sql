CREATE TABLE IF NOT EXISTS `p13_union_fight_settlement_offers` (
  `match_id` CHAR(64) NOT NULL,
  `rules` LONGBLOB NOT NULL,
  `rules_hash` CHAR(64) NOT NULL,
  `roster_hash` CHAR(64) NOT NULL,
  `frozen_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`match_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
