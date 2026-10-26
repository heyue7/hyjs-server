-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_cutoffs (
  `id` CHAR(64) NOT NULL,
  `guild_id` VARCHAR(64) NOT NULL,
  `month` CHAR(7) NOT NULL,
  `cutoff_at` BIGINT NOT NULL,
  `captured_at` BIGINT NOT NULL,
  `level` INT NOT NULL,
  `score` BIGINT NOT NULL,
  `qualified` TINYINT(1) NOT NULL,
  `rule_hash` CHAR(64) NOT NULL,
  `head_hash` CHAR(64) NOT NULL,
  `head_raw` LONGBLOB NOT NULL,
  `last_credit_id` CHAR(64) NOT NULL,
  `last_credit_hash` CHAR(64) NOT NULL,
  `roster` LONGBLOB NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY idx_guild_gift_cutoff (`guild_id`,`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
