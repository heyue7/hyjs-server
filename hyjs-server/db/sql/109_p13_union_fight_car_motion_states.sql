CREATE TABLE IF NOT EXISTS p13_union_fight_car_motion_states (
 match_id VARCHAR(64) NOT NULL,factory_guid BIGINT NOT NULL,car_guid BIGINT NOT NULL,distance DOUBLE NOT NULL,last_at BIGINT NOT NULL,arrived TINYINT(1) NOT NULL DEFAULT 0,offer_hash CHAR(64) NOT NULL,state_hash CHAR(64) NOT NULL,PRIMARY KEY(match_id,factory_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
