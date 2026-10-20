CREATE TABLE IF NOT EXISTS `p13_guild_donate_box_policies` (
 `id` bigint NOT NULL,
 `enabled` tinyint(1) NOT NULL DEFAULT 0,
 `version` varchar(128) NOT NULL,
 `source` varchar(1024) NOT NULL,
 `rules_json` text,
 `published_at` bigint NOT NULL DEFAULT 0,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
