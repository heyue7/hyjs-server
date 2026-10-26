-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_archives (
  `guild_id` VARCHAR(64) NOT NULL,
  `score` INT NOT NULL,
  `guild_raw` LONGBLOB NOT NULL,
  `guild_hash` CHAR(64) NOT NULL,
  `collected_at` BIGINT NOT NULL,
  `source` TEXT NOT NULL,
  `source_hash` CHAR(64) NOT NULL,
  PRIMARY KEY (`guild_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
