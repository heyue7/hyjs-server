CREATE TABLE IF NOT EXISTS p13_union_fight_speed_item_offers (
 match_id VARCHAR(64) NOT NULL,
 item_id VARCHAR(64) NOT NULL,
 multiplier DOUBLE NOT NULL,
 authority_mask BIGINT NOT NULL,
 radius DOUBLE NOT NULL,
 duration_millis BIGINT NOT NULL,
 cooldown_millis BIGINT NOT NULL,
 source TEXT NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(match_id,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
