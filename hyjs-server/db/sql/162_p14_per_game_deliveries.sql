CREATE TABLE IF NOT EXISTS `p14_per_game_deliveries` (
  `id` CHAR(64) NOT NULL,
  `destination_server_id` BIGINT NOT NULL,
  `payload` LONGBLOB NOT NULL,
  `payload_hash` CHAR(64) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `confirmed` TINYINT(1) NOT NULL DEFAULT 0,
  `confirmed_at` BIGINT NOT NULL,
  `attempt_count` BIGINT NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_p14_per_game_delivery_pending` (`destination_server_id`,`confirmed`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
