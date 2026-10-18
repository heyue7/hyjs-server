-- P5 one-table schema source; no formal account or resource seed.
CREATE TABLE IF NOT EXISTS `p5_mutation_receipts` (
  `player_id` bigint NOT NULL,
  `operation` varchar(64) NOT NULL,
  `timestamp` bigint NOT NULL,
  `request` longblob NOT NULL,
  `response` longblob NOT NULL, PRIMARY KEY (`player_id`,`operation`,`timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
