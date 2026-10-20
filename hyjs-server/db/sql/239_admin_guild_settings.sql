CREATE TABLE IF NOT EXISTS `admin_guild_settings` (
 `id` bigint NOT NULL,
 `settings` longtext NOT NULL,
 `version` char(64) NOT NULL DEFAULT '',
 `updated_at` bigint NOT NULL DEFAULT 0,
 `updated_by` varchar(64) NOT NULL DEFAULT '',
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
