CREATE TABLE IF NOT EXISTS p7_rush_addition_offers (
 player_id BIGINT NOT NULL, run_id BIGINT NOT NULL,
 first_fight_id VARBINARY(64) NOT NULL, first_start_hash VARCHAR(64) NOT NULL, started_at BIGINT NOT NULL,
 policy LONGBLOB NOT NULL, policy_hash VARCHAR(64) NOT NULL,
 stage_rules LONGBLOB NOT NULL, stage_hash VARCHAR(64) NOT NULL,
 selected VARCHAR(64) NOT NULL DEFAULT '', spent_stars INT NOT NULL DEFAULT 0,
 selected_at BIGINT NOT NULL DEFAULT 0, selection_stamp BIGINT NOT NULL DEFAULT 0,
 acceptance_hash VARCHAR(64) NOT NULL DEFAULT '', PRIMARY KEY (player_id,run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
