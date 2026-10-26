CREATE TABLE IF NOT EXISTS p13_union_fight_factory_cycle_offers (
 match_id VARCHAR(64) NOT NULL,
 factory_guid BIGINT NOT NULL,
 craft_millis BIGINT NOT NULL,
 animation_millis BIGINT NOT NULL,
 max_generations BIGINT NOT NULL,
 max_alive BIGINT NOT NULL,
 source TEXT NOT NULL,
 source_hash CHAR(64) NOT NULL,
 rule VARCHAR(64) NOT NULL DEFAULT '',
 buffer_capacity BIGINT NOT NULL DEFAULT 0,

 PRIMARY KEY(match_id,factory_guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
