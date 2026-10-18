-- Explicit local first-game input policy. This creates no match result.
CREATE TABLE IF NOT EXISTS p14_final_game_policies (
 id BIGINT NOT NULL PRIMARY KEY, enabled BOOLEAN NOT NULL DEFAULT FALSE,
 version VARCHAR(128) NOT NULL, source VARCHAR(1024) NOT NULL, published_at BIGINT NOT NULL
) ENGINE=InnoDB;
