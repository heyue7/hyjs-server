CREATE TABLE IF NOT EXISTS p13_union_fight_car_attack_states (
 match_id VARCHAR(64) NOT NULL,
 factory_guid BIGINT NOT NULL,
 car_guid BIGINT NOT NULL,
 next_attack_at BIGINT NOT NULL,
 pending_release_at BIGINT NOT NULL,
 completed TINYINT(1) NOT NULL DEFAULT 0,
 hits BIGINT NOT NULL DEFAULT 0,
 offer_hash VARCHAR(64) NOT NULL,
 state_hash VARCHAR(64) NOT NULL,
 PRIMARY KEY(match_id,factory_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
