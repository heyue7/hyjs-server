CREATE TABLE IF NOT EXISTS p13_union_fight_revive_receipts (
 match_id CHAR(64) NOT NULL, player_id BIGINT NOT NULL, dead_ordinal BIGINT NOT NULL,
 status VARCHAR(16) NOT NULL, rule VARCHAR(64) NOT NULL,
 death_kind VARCHAR(16) NOT NULL, death_id VARCHAR(128) NOT NULL, death_source_hash CHAR(64) NOT NULL,
 death_at BIGINT NOT NULL, due_at BIGINT NOT NULL, applied_at BIGINT NOT NULL,
 `before` BLOB NOT NULL, `after` BLOB NOT NULL, before_hash CHAR(64) NOT NULL, after_hash CHAR(64) NOT NULL,
 before_position BLOB NOT NULL, after_position BLOB NOT NULL,
 before_position_hash CHAR(64) NOT NULL, after_position_hash CHAR(64) NOT NULL,
 match_hash CHAR(64) NOT NULL, field_hash CHAR(64) NOT NULL, battle_hash CHAR(64) NOT NULL, source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(match_id,player_id,dead_ordinal)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
