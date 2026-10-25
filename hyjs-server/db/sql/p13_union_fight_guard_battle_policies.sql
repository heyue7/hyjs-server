CREATE TABLE IF NOT EXISTS `p13_union_fight_guard_battle_policies` (
 `id` BIGINT NOT NULL,
 `enabled` BOOLEAN NOT NULL DEFAULT FALSE,
 `version` VARCHAR(64) NOT NULL,
 `source` TEXT NOT NULL,
 `published_at` BIGINT NOT NULL DEFAULT 0,
 `starts_at` BIGINT NOT NULL DEFAULT 0,
 `ends_at` BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
