CREATE TABLE IF NOT EXISTS admin_opening_receipts (
 player_id BIGINT NOT NULL, timestamp BIGINT NOT NULL, occurrence_id VARCHAR(64) NOT NULL,
 request LONGBLOB NOT NULL, response LONGBLOB NOT NULL,
 PRIMARY KEY(player_id,timestamp), INDEX idx_admin_opening_receipt_period(occurrence_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
