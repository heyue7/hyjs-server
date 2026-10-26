CREATE TABLE IF NOT EXISTS p13_union_fight_tower_sources (
 player_attack BIGINT NOT NULL DEFAULT 0,
 player_damage_rule VARCHAR(64) NOT NULL DEFAULT '',
 day CHAR(10) NOT NULL,
 tower_id BIGINT NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 version VARCHAR(64) NOT NULL,
 hp_percent BIGINT NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day,tower_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
