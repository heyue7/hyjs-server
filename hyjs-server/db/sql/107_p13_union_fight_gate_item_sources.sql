CREATE TABLE IF NOT EXISTS p13_union_fight_gate_item_sources (budget_rule VARCHAR(64) NOT NULL DEFAULT '',team_use_limit BIGINT NOT NULL DEFAULT 0,target_cooldown_millis BIGINT NOT NULL DEFAULT 0,effect_family VARCHAR(32) NOT NULL DEFAULT '',
 day CHAR(10) NOT NULL, item_id VARCHAR(64) NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT FALSE, hp_percent BIGINT NOT NULL,
 published_at BIGINT NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY(day,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
