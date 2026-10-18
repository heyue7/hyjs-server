CREATE TABLE IF NOT EXISTS p13_union_fight_resource_sources (
    day CHAR(10) NOT NULL,
    source_key VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 0,
    resource_id VARCHAR(64) NOT NULL,
    x FLOAT NOT NULL,
    y FLOAT NOT NULL,
    z FLOAT NOT NULL,
    reward_res BIGINT NOT NULL,
    published_at BIGINT NOT NULL,
    source TEXT NOT NULL,
    PRIMARY KEY (day, source_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
