CREATE TABLE IF NOT EXISTS p13_union_fight_car_generations (
 match_id VARCHAR(64) NOT NULL,
 car_guid BIGINT NOT NULL,
 factory_guid BIGINT NOT NULL,
 generation BIGINT NOT NULL,
 born_at BIGINT NOT NULL,
 factory_snapshot LONGBLOB NOT NULL,
 offer_hash CHAR(64) NOT NULL,
 source_hash CHAR(64) NOT NULL,
 car_frame LONGBLOB NOT NULL,
 car_hash CHAR(64) NOT NULL,
 state_hash CHAR(64) NOT NULL,
 produced_at BIGINT NOT NULL DEFAULT 0,

 PRIMARY KEY(match_id,car_guid),
 UNIQUE KEY p13_car_generation(match_id,factory_guid,generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
