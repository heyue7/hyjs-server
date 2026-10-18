CREATE TABLE IF NOT EXISTS `p13_union_fight_transport_scopes` (
  `match_id` CHAR(64) NOT NULL,
  `started_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`match_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
