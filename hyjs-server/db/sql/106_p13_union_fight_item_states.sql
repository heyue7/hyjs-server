CREATE TABLE IF NOT EXISTS p13_union_fight_item_states (
 match_id CHAR(64) NOT NULL, player_id BIGINT NOT NULL,
 normal_until BIGINT NOT NULL DEFAULT 0, diamond_until BIGINT NOT NULL DEFAULT 0,
 source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(match_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
