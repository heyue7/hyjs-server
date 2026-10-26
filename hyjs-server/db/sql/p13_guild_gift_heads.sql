-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_heads (
  `guild_id` VARCHAR(64) NOT NULL,
  `month` CHAR(7) NOT NULL,
  `due_at` BIGINT NOT NULL,
  `score` BIGINT NOT NULL,
  `credit_count` BIGINT NOT NULL,
  `last_credit_id` CHAR(64) NOT NULL,
  `last_credit_hash` CHAR(64) NOT NULL,
  `last_credit_at` BIGINT NOT NULL,
  `rule_hash` CHAR(64) NOT NULL,
  `archive_hash` CHAR(64) NOT NULL,
  `retired_at` BIGINT NOT NULL DEFAULT 0,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`guild_id`),
  KEY idx_guild_gift_due (`due_at`,`guild_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
