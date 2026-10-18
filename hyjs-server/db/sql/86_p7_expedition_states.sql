-- 有限远征冻结原时长/耐力/经验与明确本地常规奖，默认关闭，派遣与领奖沿角色行锁。
CREATE TABLE IF NOT EXISTS p7_expedition_states (player_id BIGINT NOT NULL,level VARCHAR(8) NOT NULL,exp INT NOT NULL,day CHAR(10) NOT NULL,refresh_used INT NOT NULL,PRIMARY KEY(player_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
