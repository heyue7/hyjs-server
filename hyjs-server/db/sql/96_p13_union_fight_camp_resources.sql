CREATE TABLE IF NOT EXISTS p13_union_fight_camp_resources (
    match_id CHAR(64) NOT NULL,
    guild_id VARCHAR(64) NOT NULL,
    camp BIGINT NOT NULL,
    amount BIGINT NOT NULL DEFAULT 0,
    source TEXT NOT NULL,
    PRIMARY KEY (match_id, guild_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
