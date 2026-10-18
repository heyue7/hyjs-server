CREATE TABLE IF NOT EXISTS `p13_guild_prayer_policies` (
 `id` bigint NOT NULL,
 `enabled` tinyint(1) NOT NULL DEFAULT 0,
 `version` varchar(128) NOT NULL DEFAULT '',
 `source` varchar(1024) NOT NULL,
 `pool_json` text NOT NULL,
 `pool_hash` char(64) NOT NULL DEFAULT '',
 `published_at` bigint NOT NULL DEFAULT 0,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
