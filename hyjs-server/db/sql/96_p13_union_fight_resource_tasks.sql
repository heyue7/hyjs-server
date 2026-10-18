CREATE TABLE IF NOT EXISTS p13_union_fight_resource_tasks (
    id CHAR(64) NOT NULL,
    match_id CHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    node_guid BIGINT NOT NULL,
    started_at BIGINT NOT NULL,
    due_at BIGINT NOT NULL,
    finished_at BIGINT NOT NULL DEFAULT 0,
    status BIGINT NOT NULL DEFAULT 0,
    granted BIGINT NOT NULL DEFAULT 0,
    result BLOB NULL,
    result_hash CHAR(64) NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    KEY idx_resource_task_match(match_id),
    KEY idx_resource_task_player(player_id),
    KEY idx_resource_task_due(due_at),
    KEY idx_resource_task_pending(status,due_at,id),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
