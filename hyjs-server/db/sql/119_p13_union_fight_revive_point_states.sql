CREATE TABLE IF NOT EXISTS p13_union_fight_revive_point_states (
 match_id VARCHAR(64) NOT NULL,point_guid BIGINT NOT NULL,point LONGBLOB NOT NULL,participants LONGBLOB NOT NULL,last_at BIGINT NOT NULL,offer_hash CHAR(64) NOT NULL,state_hash CHAR(64) NOT NULL,PRIMARY KEY(match_id,point_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
