CREATE TABLE IF NOT EXISTS turntable_draw_receipts (
 player_id BIGINT NOT NULL, stamp BIGINT NOT NULL, pool_id VARCHAR(64) NOT NULL,
 count INT NOT NULL, response LONGBLOB NOT NULL, PRIMARY KEY(player_id,stamp)
);
