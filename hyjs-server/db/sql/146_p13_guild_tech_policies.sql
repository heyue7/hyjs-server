CREATE TABLE IF NOT EXISTS `p13_guild_tech_policies` (
 `id` bigint NOT NULL, `enabled` tinyint(1) NOT NULL DEFAULT 0, `version` varchar(128) NOT NULL DEFAULT '',
 `directory_json` text NOT NULL, `directory_hash` char(64) NOT NULL, `published_at` bigint NOT NULL DEFAULT 0,
 `source` varchar(1024) NOT NULL, PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
