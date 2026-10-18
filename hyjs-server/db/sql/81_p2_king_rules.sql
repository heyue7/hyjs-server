-- P2 专属 SQL 表；原玩家没有初始化赠送红卡/红装。
CREATE TABLE IF NOT EXISTS p2_king_rules (
 id BIGINT NOT NULL PRIMARY KEY, daily_limit INT, luck_limit INT, luck_per_draw INT,
 rotation_millis BIGINT, epoch_millis BIGINT, wheel_pool VARCHAR(64), source VARCHAR(255)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
