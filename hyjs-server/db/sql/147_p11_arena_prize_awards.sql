CREATE TABLE IF NOT EXISTS p11_arena_prize_awards (
 fight_id VARCHAR(128) NOT NULL PRIMARY KEY, player_id BIGINT NOT NULL,
 player_level BIGINT NOT NULL, stamp BIGINT NOT NULL, winner_id BIGINT NOT NULL, p6_record_id VARCHAR(128) NOT NULL,
 terminal_hash VARCHAR(64) NOT NULL, scope_proof VARCHAR(64) NOT NULL,
 base_response_hash VARCHAR(64) NOT NULL, final_response_hash VARCHAR(64) NOT NULL,
 month VARCHAR(6) NOT NULL, day VARCHAR(8) NOT NULL, status VARCHAR(32) NOT NULL,
 offer_hash VARCHAR(64) NOT NULL, day_ordinal BIGINT NOT NULL,
 window_ordinal BIGINT NOT NULL, draw_json TEXT NOT NULL, boxes_json TEXT NOT NULL,
 grant_pb LONGBLOB NULL, source_hash VARCHAR(64) NOT NULL, created_at BIGINT NOT NULL,
 INDEX idx_p11_arena_prize_player_month(player_id,month)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
