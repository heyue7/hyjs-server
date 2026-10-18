-- P2 专属 SQL 表；原玩家没有初始化赠送红卡/红装。
CREATE TABLE IF NOT EXISTS p2_king_states (
 player_id BIGINT NOT NULL PRIMARY KEY, day VARCHAR(10), draws INT, luck INT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
