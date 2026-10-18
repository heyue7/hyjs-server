CREATE TABLE IF NOT EXISTS p13_union_fight_car_item_offers (
 match_id CHAR(64) NOT NULL, item_id VARCHAR(64) NOT NULL,
 hp_percent BIGINT NOT NULL, radius DOUBLE NOT NULL, cooldown_millis BIGINT NOT NULL,
 source TEXT NOT NULL, source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(match_id,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
