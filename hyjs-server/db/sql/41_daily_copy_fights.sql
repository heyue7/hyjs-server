CREATE TABLE IF NOT EXISTS daily_copy_fights (
 fight_id VARCHAR(64) NOT NULL PRIMARY KEY, player_id BIGINT NOT NULL,
 start_stamp BIGINT NOT NULL, day VARCHAR(10) NOT NULL, type VARCHAR(64) NOT NULL,
 chapter_id VARCHAR(64) NOT NULL, stamina INT NOT NULL, win_stars INT NOT NULL,
 reward TEXT NOT NULL, start_snapshot LONGBLOB NOT NULL, settled BOOLEAN NOT NULL,
 end_request LONGBLOB NULL, response LONGBLOB NULL,
 UNIQUE KEY idx_daily_copy_start(player_id,start_stamp),
 KEY idx_daily_copy_active(player_id,day,settled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
