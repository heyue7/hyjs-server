-- P2 专属 SQL 表；原玩家没有初始化赠送红卡/红装。
CREATE TABLE IF NOT EXISTS p2_red_receipts (
 player_id BIGINT NOT NULL, operation VARCHAR(32) NOT NULL, timestamp BIGINT NOT NULL,
 request LONGBLOB, response LONGBLOB, PRIMARY KEY(player_id,operation,timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
