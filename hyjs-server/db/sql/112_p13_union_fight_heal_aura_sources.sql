CREATE TABLE IF NOT EXISTS p13_union_fight_heal_aura_sources (
 effect_rule VARCHAR(64) NOT NULL DEFAULT '',
 day CHAR(10) NOT NULL,
 item_id VARCHAR(64) NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 hp_percent BIGINT NOT NULL,
 radius DOUBLE NOT NULL,
 tick_millis BIGINT NOT NULL,
 authority_mask BIGINT NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
