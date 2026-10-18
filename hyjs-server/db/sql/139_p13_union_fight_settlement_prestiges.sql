CREATE TABLE IF NOT EXISTS `p13_union_fight_settlement_prestiges` (
  `week_start` VARCHAR(10) NOT NULL,
  `guild_id` CHAR(64) NOT NULL,
  `score` BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (`week_start`,`guild_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
