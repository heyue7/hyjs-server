CREATE TABLE IF NOT EXISTS p13_union_fight_speed_item_sources (
 day CHAR(10) NOT NULL,
 item_id VARCHAR(64) NOT NULL,
 enabled TINYINT(1) NOT NULL DEFAULT 0,
 multiplier DOUBLE NOT NULL,
 authority_mask BIGINT NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
