CREATE TABLE IF NOT EXISTS p11_plunder_sweep_receipts (
 player_id BIGINT NOT NULL,
 stamp BIGINT NOT NULL,
 request_hash VARCHAR(64) NOT NULL,
 fragment_id VARCHAR(64) NOT NULL,
 requested_count INT NOT NULL,
 accepted_count INT NOT NULL,
 plan LONGBLOB NOT NULL,
 source_hash VARCHAR(64) NOT NULL,
 response LONGBLOB NOT NULL,
 response_hash VARCHAR(64) NOT NULL,
 source VARCHAR(191) NOT NULL,
 created_at BIGINT NOT NULL,
 finished_at BIGINT NOT NULL,
 PRIMARY KEY(player_id,stamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
