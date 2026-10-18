CREATE TABLE IF NOT EXISTS p13_union_fight_shop_stocks (
 match_id CHAR(64) NOT NULL,
 player_id BIGINT NOT NULL,
 shop_item_id VARCHAR(64) NOT NULL,
 left_count BIGINT NOT NULL,
 last_refresh_at BIGINT NOT NULL,
 PRIMARY KEY(match_id,player_id,shop_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
