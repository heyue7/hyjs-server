CREATE TABLE IF NOT EXISTS p13_union_fight_shop_offers (
 match_id CHAR(64) NOT NULL,
 shop_item_id VARCHAR(64) NOT NULL,
 frame BLOB NOT NULL,
 frame_hash CHAR(64) NOT NULL,
 source_hash CHAR(64) NOT NULL,
 source TEXT NOT NULL,
 PRIMARY KEY(match_id,shop_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
