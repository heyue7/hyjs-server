-- P5 one-table schema source; no formal account or resource seed.
CREATE TABLE IF NOT EXISTS `p5_collections` (
  `player_id` bigint NOT NULL,
  `hero_id` varchar(64) NOT NULL,
  `acquired_at` bigint NOT NULL, PRIMARY KEY (`player_id`,`hero_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
