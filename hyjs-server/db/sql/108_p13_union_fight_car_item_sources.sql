CREATE TABLE IF NOT EXISTS p13_union_fight_car_item_sources (
 day CHAR(10) NOT NULL, item_id VARCHAR(64) NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT FALSE, hp_percent BIGINT NOT NULL,
 published_at BIGINT NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY(day,item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
