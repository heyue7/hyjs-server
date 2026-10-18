CREATE TABLE IF NOT EXISTS `p13_union_fight_player_battle_offers` (
 `match_id` CHAR(64) NOT NULL, `snapshot` LONGTEXT NOT NULL,
 `source_hash` CHAR(64) NOT NULL, `captured_at` BIGINT NOT NULL,
 PRIMARY KEY (`match_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
