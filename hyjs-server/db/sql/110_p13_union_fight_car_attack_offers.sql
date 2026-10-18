CREATE TABLE IF NOT EXISTS p13_union_fight_car_attack_offers (
 match_id VARCHAR(64) NOT NULL,
 factory_guid BIGINT NOT NULL,
 gate_guid BIGINT NOT NULL,
 gate_id BIGINT NOT NULL,
 hp_percent BIGINT NOT NULL,
 radius DOUBLE NOT NULL,
 stop_distance DOUBLE NOT NULL,
 cooldown_millis BIGINT NOT NULL,
 release_millis BIGINT NOT NULL,
 motion_hash VARCHAR(64) NOT NULL,
 source TEXT NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(match_id,factory_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
