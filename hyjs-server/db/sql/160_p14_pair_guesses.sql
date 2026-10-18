CREATE TABLE IF NOT EXISTS `p14_pair_guesses` (
  `season_index` INT NOT NULL,
  `pair_hash` CHAR(64) NOT NULL,
  `owner_server_id` BIGINT NOT NULL,
  `owner_player_id` BIGINT NOT NULL,
  `target_server_id` BIGINT NOT NULL,
  `target_player_id` BIGINT NOT NULL,
  `offer_hash` CHAR(64) NOT NULL,
  `cost_coin` BIGINT NOT NULL,
  `coin_before` BIGINT NOT NULL,
  `coin_after` BIGINT NOT NULL,
  `captured_at` BIGINT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`season_index`, `pair_hash`, `owner_server_id`, `owner_player_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
