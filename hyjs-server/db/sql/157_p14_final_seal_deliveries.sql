CREATE TABLE IF NOT EXISTS p14_final_seal_deliveries (
 id CHAR(64) NOT NULL PRIMARY KEY, destination_server_id BIGINT NOT NULL,
 payload LONGBLOB NOT NULL, payload_hash CHAR(64) NOT NULL, created_at BIGINT NOT NULL,
 confirmed BOOLEAN NOT NULL DEFAULT FALSE, confirmed_at BIGINT NOT NULL DEFAULT 0,
 attempt_count BIGINT NOT NULL DEFAULT 0,
 KEY idx_p14_final_seal_peer(destination_server_id), KEY idx_p14_final_seal_confirmed(confirmed)
) ENGINE=InnoDB;
