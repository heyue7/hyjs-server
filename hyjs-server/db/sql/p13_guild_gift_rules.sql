-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_rules (
  `month` CHAR(7) NOT NULL,
  `start_at` BIGINT NOT NULL,
  `end_at` BIGINT NOT NULL,
  `credit` INT NOT NULL,
  `levels` LONGBLOB NOT NULL,
  `levels_hash` CHAR(64) NOT NULL,
  `config` LONGBLOB NOT NULL,
  `config_hash` CHAR(64) NOT NULL,
  `login_config` LONGBLOB NOT NULL,
  `login_config_hash` CHAR(64) NOT NULL,
  `frozen_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
