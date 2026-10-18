CREATE TABLE IF NOT EXISTS p7_awake_explorations (
 run_id VARBINARY(64) PRIMARY KEY,
 player_id BIGINT NOT NULL,
 status VARCHAR(16) NOT NULL,
 state LONGBLOB NOT NULL,
 state_hash CHAR(64) NOT NULL,
 enter_stamp BIGINT NOT NULL,
 settle_stamp BIGINT NOT NULL,
 settled_at BIGINT NOT NULL,
 receipt LONGBLOB NULL,
 receipt_hash CHAR(64) NOT NULL,
 INDEX idx_awake_player_status(player_id,status),
 INDEX idx_awake_player_settle_stamp(player_id,settle_stamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
