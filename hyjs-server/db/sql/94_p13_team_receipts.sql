-- P13独立野外队伍实际管理事实。
CREATE TABLE IF NOT EXISTS p13_team_receipts (player_id BIGINT NOT NULL,operation VARCHAR(64) NOT NULL,stamp BIGINT NOT NULL,request LONGBLOB NOT NULL,response LONGBLOB NOT NULL,created_at BIGINT NOT NULL,PRIMARY KEY(player_id,operation,stamp)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
