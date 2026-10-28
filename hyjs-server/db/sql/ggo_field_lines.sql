CREATE TABLE IF NOT EXISTS ggo_field_lines (
 server_id BIGINT NOT NULL,
 line BIGINT NOT NULL,
 revision BIGINT NOT NULL,
 payload LONGBLOB NOT NULL,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY (server_id, line)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
