CREATE TABLE IF NOT EXISTS ggo_field_lines (
 server_id BIGINT NOT NULL,
 line BIGINT NOT NULL,
 revision BIGINT NOT NULL,
 payload LONGBLOB NOT NULL,
 updated_at BIGINT NOT NULL,
 next_due_at BIGINT NOT NULL DEFAULT 0,
 point_state_ready BOOLEAN NOT NULL DEFAULT FALSE,
 deployment_revision VARCHAR(128) NOT NULL DEFAULT '',
 cycle_ms BIGINT NOT NULL DEFAULT 0,
 generation_floor BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY (server_id, line),
 KEY idx_ggo_lines_due (server_id, next_due_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
