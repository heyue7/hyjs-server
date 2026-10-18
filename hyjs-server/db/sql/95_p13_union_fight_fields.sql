CREATE TABLE IF NOT EXISTS p13_union_fight_fields (
 match_id CHAR(64) NOT NULL,snapshot LONGBLOB NOT NULL,snapshot_hash CHAR(64) NOT NULL,created_at BIGINT NOT NULL,source TEXT NOT NULL,
 PRIMARY KEY(match_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
