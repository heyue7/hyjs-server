CREATE TABLE IF NOT EXISTS p13_union_fight_war_drum_sources (
 day CHAR(10) NOT NULL,
 item_id VARCHAR(64) NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 attack_percent BIGINT NOT NULL,
 radius DOUBLE NOT NULL,
 duration_millis BIGINT NOT NULL,
 authority_mask BIGINT NOT NULL,
 model_name VARCHAR(128) NOT NULL,
 model_scale DOUBLE NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
