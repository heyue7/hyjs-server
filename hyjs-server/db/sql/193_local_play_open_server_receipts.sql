CREATE TABLE IF NOT EXISTS local_play_open_server_receipts (
 player_id BIGINT NOT NULL, timestamp BIGINT NOT NULL, cycle_id VARCHAR(64) NOT NULL, request LONGBLOB NOT NULL, response LONGBLOB NOT NULL, PRIMARY KEY(player_id,timestamp), KEY idx_local_play_os_receipt_cycle(cycle_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
