-- P2 专属 SQL 表；原玩家没有初始化赠送红卡/红装。
CREATE TABLE IF NOT EXISTS p2_red_box_rewards (
 box_id VARCHAR(64) NOT NULL, resource_id VARCHAR(64) NOT NULL, amount INT, weight BIGINT,
 source VARCHAR(255), PRIMARY KEY(box_id,resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
