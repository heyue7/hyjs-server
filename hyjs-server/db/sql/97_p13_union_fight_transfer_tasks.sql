CREATE TABLE IF NOT EXISTS p13_union_fight_transfer_tasks (
 id CHAR(64) NOT NULL, match_id CHAR(64) NOT NULL, player_id BIGINT NOT NULL,
 ordinal BIGINT NOT NULL, transfer_id VARCHAR(64) NOT NULL, target BLOB NOT NULL,
 started_at BIGINT NOT NULL, due_at BIGINT NOT NULL, finished_at BIGINT NOT NULL DEFAULT 0,
 status BIGINT NOT NULL DEFAULT 0, source_hash CHAR(64) NOT NULL,
 result BLOB NULL, result_hash CHAR(64) NOT NULL DEFAULT '', source TEXT NOT NULL,
 PRIMARY KEY(id), KEY idx_transfer_match(match_id), KEY idx_transfer_player(player_id),
 KEY idx_uf_transfer_pending(status,due_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
