CREATE TABLE IF NOT EXISTS `p11_grab_fight_policies` (
 `id` BIGINT NOT NULL,
 `enabled` BOOLEAN NOT NULL,
 `version` VARCHAR(64) NOT NULL,
 `source` TEXT NOT NULL,
 `published_at` BIGINT NOT NULL,
 `starts_at` BIGINT NOT NULL,
 `ends_at` BIGINT NOT NULL,
 `max_round` INT NOT NULL,
 `attacker_first` BOOLEAN NOT NULL,
 PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
