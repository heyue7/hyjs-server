CREATE TABLE IF NOT EXISTS `story_quest_receipts` (
 `player_id` bigint NOT NULL,
 `stamp` bigint NOT NULL,
 `quest_id` varchar(64) NOT NULL,
 `request` blob NOT NULL,
 `response` longblob NOT NULL,
 `created_at` bigint NOT NULL,
 `expires_at` bigint NOT NULL,
 `version` varchar(128) NOT NULL,
 `source` varchar(1024) NOT NULL,
 `source_hash` char(64) NOT NULL,
 PRIMARY KEY (`player_id`,`stamp`),
 UNIQUE KEY `idx_story_quest_claim` (`player_id`,`quest_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
