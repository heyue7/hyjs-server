CREATE TABLE IF NOT EXISTS turntable_personals (
 pool_id VARCHAR(64) NOT NULL, player_id BIGINT NOT NULL,
 diamond_pool BIGINT NOT NULL, contribution_remainder BIGINT NOT NULL DEFAULT 0,
 legacy_score INT NOT NULL DEFAULT 0, used_stock LONGBLOB, settled BOOLEAN NOT NULL DEFAULT FALSE, created_at BIGINT NOT NULL,
 PRIMARY KEY(pool_id,player_id)
);
