CREATE TABLE IF NOT EXISTS p13_union_fight_gate_impacts (
 id CHAR(64) NOT NULL,match_id CHAR(64) NOT NULL,gate_guid BIGINT NOT NULL,ordinal BIGINT NOT NULL,
 kind VARCHAR(16) NOT NULL,offer_hash CHAR(64) NOT NULL,player_id BIGINT NOT NULL DEFAULT 0,stamp BIGINT NOT NULL DEFAULT 0,
 item_id VARCHAR(32) NOT NULL DEFAULT '',inventory_before INT NOT NULL DEFAULT 0,inventory_after INT NOT NULL DEFAULT 0,
 car_guid BIGINT NOT NULL DEFAULT 0,hit_ordinal BIGINT NOT NULL DEFAULT 0,percent BIGINT NOT NULL,
 logical_at BIGINT NOT NULL,applied_at BIGINT NOT NULL,`before` BLOB NOT NULL,`after` BLOB NOT NULL,
 before_hash CHAR(64) NOT NULL,after_hash CHAR(64) NOT NULL,source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(id),UNIQUE KEY p13_gate_impact_order(match_id,gate_guid,ordinal),KEY p13_gate_impact_time(match_id,logical_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
