CREATE TABLE IF NOT EXISTS p13_union_fight_factory_cycle_states (
 match_id VARCHAR(64) NOT NULL,
 factory_guid BIGINT NOT NULL,
 next_born_at BIGINT NOT NULL,
 generation BIGINT NOT NULL,
 animation_played TINYINT(1) NOT NULL DEFAULT 0,
 closed_at BIGINT NOT NULL DEFAULT 0,
 offer_hash CHAR(64) NOT NULL,
 state_hash CHAR(64) NOT NULL,
 PRIMARY KEY(match_id,factory_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
