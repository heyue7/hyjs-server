-- Ordinary guild monthly gifts; no war ranking or retroactive eligibility.
CREATE TABLE IF NOT EXISTS p13_guild_gift_policies (
  `id` BIGINT NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `starts_at` BIGINT NOT NULL,
  `published_at` BIGINT NOT NULL,
  `rule` VARCHAR(96) NOT NULL,
  `source` TEXT NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
