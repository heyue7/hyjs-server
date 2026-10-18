-- 原跨服联合身份、精确回执与持久投递；无胜负和奖励。
CREATE TABLE IF NOT EXISTS p14_receipts (player_id BIGINT NOT NULL,operation VARCHAR(64) NOT NULL,stamp BIGINT NOT NULL,request LONGBLOB NOT NULL,response LONGBLOB NOT NULL,PRIMARY KEY(player_id,operation,stamp)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
