-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_awards (
  `id` CHAR(64) NOT NULL,
  `player_id` BIGINT NOT NULL,
  `guild_id` VARCHAR(64) NOT NULL,
  `month` CHAR(7) NOT NULL,
  `role` INT NOT NULL,
  `cutoff_id` CHAR(64) NOT NULL,
  `cutoff_hash` CHAR(64) NOT NULL,
  `rewards` TEXT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  `delivered_at` BIGINT NOT NULL DEFAULT 0,
  `delivery_id` BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY idx_guild_gift_outbox (`delivered_at`,`id`),
  KEY idx_guild_gift_person (`player_id`,`delivered_at`),
  UNIQUE KEY idx_guild_gift_award (`player_id`,`guild_id`,`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
