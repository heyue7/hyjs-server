CREATE TABLE IF NOT EXISTS p13_union_fight_car_motion_offers (
 match_id VARCHAR(64) NOT NULL,factory_guid BIGINT NOT NULL,factory_id BIGINT NOT NULL,speed DOUBLE NOT NULL,path LONGBLOB NOT NULL,source TEXT NOT NULL,source_hash CHAR(64) NOT NULL,PRIMARY KEY(match_id,factory_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
