CREATE TABLE IF NOT EXISTS admin_field_channel_activations (
 server_id BIGINT NOT NULL PRIMARY KEY,
 version BIGINT NOT NULL,
 source TEXT NOT NULL
);
