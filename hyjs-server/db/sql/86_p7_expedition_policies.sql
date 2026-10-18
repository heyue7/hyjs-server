-- 有限远征冻结原时长/耐力/经验与明确本地常规奖，默认关闭，派遣与领奖沿角色行锁。
CREATE TABLE IF NOT EXISTS p7_expedition_policies (id BIGINT NOT NULL,enabled BOOLEAN NOT NULL,reward TEXT NOT NULL,source TEXT NOT NULL,PRIMARY KEY(id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
