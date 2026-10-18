CREATE TABLE IF NOT EXISTS p13_union_fight_loadouts (
  match_id CHAR(64) NOT NULL,
  player_id BIGINT NOT NULL,
  item_ids VARCHAR(512) NOT NULL,
  prepared_at BIGINT NOT NULL,
  PRIMARY KEY(match_id,player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
