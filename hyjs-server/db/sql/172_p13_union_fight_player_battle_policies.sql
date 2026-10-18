CREATE TABLE IF NOT EXISTS `p13_union_fight_player_battle_policies` (
 `id` BIGINT NOT NULL, `enabled` BOOLEAN NOT NULL DEFAULT FALSE,
 `version` VARCHAR(64) NOT NULL, `source` TEXT NOT NULL,
 `published_at` BIGINT NOT NULL, `starts_at` BIGINT NOT NULL, `ends_at` BIGINT NOT NULL,
 `revive_rule` VARCHAR(64) NOT NULL,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
