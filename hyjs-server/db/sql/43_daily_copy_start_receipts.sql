CREATE TABLE IF NOT EXISTS daily_copy_start_receipts (
 player_id BIGINT NOT NULL, stamp BIGINT NOT NULL,
 chapter_id VARCHAR(64) NOT NULL, fight_id VARCHAR(64) NOT NULL,
 PRIMARY KEY(player_id,stamp), KEY idx_daily_copy_start_receipts_fight_id(fight_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
