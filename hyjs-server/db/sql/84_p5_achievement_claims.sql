-- P5 one-table schema source; no formal account or resource seed.
CREATE TABLE IF NOT EXISTS `p5_achievement_claims` (
  `player_id` bigint NOT NULL,
  `id` varchar(64) NOT NULL,
  `snapshot` longtext NOT NULL,
  `progress` int NOT NULL DEFAULT 0,
  `claimed_at` bigint NOT NULL, PRIMARY KEY (`player_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
