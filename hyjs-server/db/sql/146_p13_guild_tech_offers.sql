CREATE TABLE IF NOT EXISTS `p13_guild_tech_offers` (
 `guild_id` varchar(64) NOT NULL, `version` varchar(128) NOT NULL, `directory_json` text NOT NULL, `directory_hash` char(64) NOT NULL,
 `original_rows_json` text NOT NULL, `original_rows_hash` char(64) NOT NULL, `original_client` varchar(128) NOT NULL,
 `published_at` bigint NOT NULL, `captured_at` bigint NOT NULL, `source` varchar(1024) NOT NULL, `source_hash` char(64) NOT NULL,
 PRIMARY KEY (`guild_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
