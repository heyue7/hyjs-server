-- P2 专属 SQL 表；原玩家没有初始化赠送红卡/红装。
CREATE TABLE IF NOT EXISTS p2_red_exchange_states (
 player_id BIGINT NOT NULL, product_id INT NOT NULL, day VARCHAR(10) NOT NULL,
 bought INT, PRIMARY KEY(player_id,product_id,day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
