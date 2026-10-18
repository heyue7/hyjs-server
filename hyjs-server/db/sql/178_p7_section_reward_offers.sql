CREATE TABLE IF NOT EXISTS p7_section_reward_offers (
 player_id BIGINT NOT NULL,run_id BIGINT NOT NULL,
 first_fight_id VARCHAR(64) NOT NULL,first_start_hash VARCHAR(64) NOT NULL,
 started_at BIGINT NOT NULL,policy LONGBLOB NOT NULL,policy_hash VARCHAR(64) NOT NULL,
 stage_rules LONGBLOB NOT NULL,claimed_at BIGINT NOT NULL DEFAULT 0,claim_stamp BIGINT NOT NULL DEFAULT 0,
 claimed_stars INT NOT NULL DEFAULT 0,acceptance_hash VARCHAR(64) NOT NULL DEFAULT '',
 PRIMARY KEY(player_id,run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
