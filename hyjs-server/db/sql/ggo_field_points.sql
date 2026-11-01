CREATE TABLE IF NOT EXISTS ggo_field_points (
 server_id BIGINT NOT NULL,
 line BIGINT NOT NULL,
 point_id VARBINARY(64) NOT NULL,
 revision BIGINT NOT NULL,
 payload LONGBLOB NOT NULL,
 monster_uuid VARCHAR(64) NOT NULL DEFAULT '',
 next_due_at BIGINT NOT NULL,
 PRIMARY KEY (server_id, line, point_id),
 KEY idx_ggo_point_monster (server_id, line, monster_uuid),
 KEY idx_ggo_point_due (server_id, next_due_at, line, point_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
