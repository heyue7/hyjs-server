-- P2 专属 SQL 表；原玩家没有初始化赠送红卡/红装。
CREATE TABLE IF NOT EXISTS p2_red_exchanges (
 id INT NOT NULL PRIMARY KEY, resource_id VARCHAR(64), amount INT, cost VARCHAR(128),
 daily_limit INT, stock INT, used INT, min_level INT, enabled BOOLEAN, source VARCHAR(255)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
