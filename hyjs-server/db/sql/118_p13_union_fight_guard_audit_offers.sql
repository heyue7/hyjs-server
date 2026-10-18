CREATE TABLE IF NOT EXISTS p13_union_fight_guard_audit_offers (
 match_id VARCHAR(64) NOT NULL,guard_guid BIGINT NOT NULL,guard_id VARCHAR(64) NOT NULL,hero_id VARCHAR(64) NOT NULL,camp BIGINT NOT NULL,bind_guid BIGINT NOT NULL,initial_hp DOUBLE NOT NULL,arrive_range DOUBLE NOT NULL,monster LONGBLOB NOT NULL,monster_hash CHAR(64) NOT NULL,battle_offer_hash CHAR(64) NOT NULL,source TEXT NOT NULL,source_hash CHAR(64) NOT NULL,PRIMARY KEY(match_id,guard_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
