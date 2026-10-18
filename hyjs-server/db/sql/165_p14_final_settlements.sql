CREATE TABLE IF NOT EXISTS `p14_final_settlements` (
  `series_id` CHAR(64) NOT NULL,
  `game_index` INT NOT NULL,
  `offer_hash` CHAR(64) NOT NULL,
  `input_id` CHAR(64) NOT NULL,
  `input_hash` CHAR(64) NOT NULL,
  `provenance_hash` CHAR(64) NOT NULL,
  `p6_record_id` CHAR(64) NOT NULL,
  `scope_proof` CHAR(64) NOT NULL,
  `winner_camp` INT NOT NULL,
  `winner_server_id` BIGINT NOT NULL,
  `winner_player_id` BIGINT NOT NULL,
  `terminal` LONGBLOB NOT NULL,
  `terminal_hash` CHAR(64) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`series_id`,`game_index`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
