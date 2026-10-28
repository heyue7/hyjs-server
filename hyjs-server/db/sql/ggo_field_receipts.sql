CREATE TABLE IF NOT EXISTS ggo_field_receipts (
 player_id BIGINT NOT NULL,
 request_id VARBINARY(36) NOT NULL,
 operation VARBINARY(32) NOT NULL,
 business_key VARBINARY(128) NULL,
 request_hash CHAR(64) NOT NULL,
 response LONGBLOB NOT NULL,
 counterparty_id BIGINT NOT NULL,
 economy_version VARBINARY(64) NOT NULL,
 chip_delta BLOB NOT NULL,
 created_at BIGINT NOT NULL,
 active BOOLEAN NOT NULL DEFAULT FALSE,
 ready_at BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY (player_id, request_id),
 UNIQUE KEY uidx_ggo_field_business (player_id, operation, business_key),
 KEY idx_ggo_field_due (operation, active, ready_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
