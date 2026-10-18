CREATE TABLE IF NOT EXISTS p13_union_fight_war_drum_offers (
 match_id VARCHAR(64) NOT NULL,
 item_id VARCHAR(64) NOT NULL,
 attack_percent BIGINT NOT NULL,
 radius DOUBLE NOT NULL,
 duration_millis BIGINT NOT NULL,
 authority_mask BIGINT NOT NULL,
 cooldown_millis BIGINT NOT NULL,
 model_name VARCHAR(128) NOT NULL,
 model_scale DOUBLE NOT NULL,
 source TEXT NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(match_id,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
