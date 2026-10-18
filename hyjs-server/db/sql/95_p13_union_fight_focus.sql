CREATE TABLE IF NOT EXISTS p13_union_fight_focus (
    match_id CHAR(64) NOT NULL,
    guild_id VARCHAR(64) NOT NULL,
    leader_id BIGINT NOT NULL,
    point BLOB NOT NULL,
    point_hash CHAR(64) NOT NULL,
    published_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    ready_at BIGINT NOT NULL,
    cleared_at BIGINT NOT NULL DEFAULT 0,
    source TEXT NOT NULL,
    PRIMARY KEY (match_id, guild_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
