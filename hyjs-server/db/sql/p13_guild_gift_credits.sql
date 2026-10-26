-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_credits (
  `id` CHAR(64) NOT NULL,
  `player_id` BIGINT NOT NULL,
  `day` CHAR(10) NOT NULL,
  `guild_id` VARCHAR(64) NOT NULL,
  `month` CHAR(7) NOT NULL,
  `joined_at` BIGINT NOT NULL,
  `amount` INT NOT NULL,
  `before_score` BIGINT NOT NULL,
  `after_score` BIGINT NOT NULL,
  `previous_id` CHAR(64) NOT NULL,
  `previous_hash` CHAR(64) NOT NULL,
  `rule_hash` CHAR(64) NOT NULL,
  `occurred_at` BIGINT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY idx_guild_gift_player_day (`player_id`,`day`),
  KEY idx_guild_gift_credits (`guild_id`,`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
