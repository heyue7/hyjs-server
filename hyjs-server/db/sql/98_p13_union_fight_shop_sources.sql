CREATE TABLE IF NOT EXISTS p13_union_fight_shop_sources (
 day CHAR(10) NOT NULL,
 shop_item_id VARCHAR(64) NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT 0,
 item_id VARCHAR(64) NOT NULL,
 price BIGINT NOT NULL,
 cost_resource VARCHAR(64) NOT NULL DEFAULT '',
 quantity BIGINT NOT NULL,
 buy_count BIGINT NOT NULL,
 buy_condition BIGINT NOT NULL,
 priority BIGINT NOT NULL,
 refresh_millis BIGINT NOT NULL,
 published_at BIGINT NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(day,shop_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
