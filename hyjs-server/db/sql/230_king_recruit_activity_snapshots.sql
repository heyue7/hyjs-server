CREATE TABLE IF NOT EXISTS king_recruit_activity_snapshots (
    pool_id VARCHAR(64) NOT NULL,
    payload LONGBLOB NOT NULL,
    frozen BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (pool_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
