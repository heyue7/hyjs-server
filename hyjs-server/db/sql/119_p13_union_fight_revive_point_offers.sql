CREATE TABLE IF NOT EXISTS p13_union_fight_revive_point_offers (
 match_id VARCHAR(64) NOT NULL,point_guid BIGINT NOT NULL,point_id BIGINT NOT NULL,x FLOAT NOT NULL,z FLOAT NOT NULL,radius DOUBLE NOT NULL,speed DOUBLE NOT NULL,max_step_millis BIGINT NOT NULL,max_gap_millis BIGINT NOT NULL,source TEXT NOT NULL,source_hash CHAR(64) NOT NULL,PRIMARY KEY(match_id,point_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
