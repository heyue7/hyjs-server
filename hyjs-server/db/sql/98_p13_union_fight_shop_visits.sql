CREATE TABLE IF NOT EXISTS p13_union_fight_shop_visits (
 match_id CHAR(64) NOT NULL,
 player_id BIGINT NOT NULL,
 npc_id VARCHAR(64) NOT NULL,
 visited_at BIGINT NOT NULL,
 PRIMARY KEY(match_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
