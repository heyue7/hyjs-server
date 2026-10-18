CREATE TABLE IF NOT EXISTS p13_union_fight_resource_nodes (
    match_id CHAR(64) NOT NULL,
    guid BIGINT NOT NULL,
    frame BLOB NOT NULL,
    frame_hash CHAR(64) NOT NULL,
    source_hash CHAR(64) NOT NULL,
    x FLOAT NOT NULL,
    y FLOAT NOT NULL,
    z FLOAT NOT NULL,
    total_uses BIGINT NOT NULL,
    remaining_uses BIGINT NOT NULL,
    gather_millis BIGINT NOT NULL,
    busy_task_id CHAR(64) NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    PRIMARY KEY (match_id, guid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
