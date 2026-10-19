CREATE TABLE IF NOT EXISTS admin_field_channels (
 server_id BIGINT NOT NULL,
 scene VARCHAR(64) NOT NULL,
 line_count BIGINT NOT NULL,
 max_players BIGINT NOT NULL,
 revision BIGINT NOT NULL,
 PRIMARY KEY (server_id, scene)
);
