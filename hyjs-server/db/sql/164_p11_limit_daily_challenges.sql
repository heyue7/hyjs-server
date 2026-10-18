CREATE TABLE IF NOT EXISTS `p11_limit_daily_challenges` (
 `id` BIGINT NOT NULL AUTO_INCREMENT,
 `fight_id` VARCHAR(64) NOT NULL,
 `period_id` VARCHAR(64) NOT NULL,
 `player_id` BIGINT NOT NULL,
 `opponent_id` BIGINT NOT NULL,
 `accepted_at` BIGINT NOT NULL,
 `response_hash` VARCHAR(64) NOT NULL,
 `terminal_hash` VARCHAR(64) NOT NULL,
 `p6_record_id` VARCHAR(64) NOT NULL,
 `scope_proof` VARCHAR(64) NOT NULL,
 `source_hash` VARCHAR(64) NOT NULL,
 PRIMARY KEY (`id`),
 UNIQUE KEY `uq_fight_id` (`fight_id`),
 KEY `idx_period_id` (`period_id`),
 KEY `idx_player_id` (`player_id`),
 KEY `idx_accepted_at` (`accepted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
