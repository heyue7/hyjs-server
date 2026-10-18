CREATE TABLE IF NOT EXISTS `p14_final_continuation_deliveries` (
  `series_id` CHAR(64) NOT NULL,
  `game_index` INT NOT NULL,
  `source_result_hash` CHAR(64) NOT NULL,
  `id` CHAR(64) NOT NULL,
  `destination_server_id` BIGINT NOT NULL,
  `payload` LONGBLOB NOT NULL,
  `payload_hash` CHAR(64) NOT NULL,
  `confirmed` TINYINT(1) NOT NULL,
  `attempt_count` BIGINT NOT NULL,
  `created_at` BIGINT NOT NULL,
  `last_attempt_at` BIGINT NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_p14_final_continuation_destination` (`series_id`,`game_index`,`destination_server_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
