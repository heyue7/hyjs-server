-- 有限远征冻结原时长/耐力/经验与明确本地常规奖，默认关闭，派遣与领奖沿角色行锁。
CREATE TABLE IF NOT EXISTS p7_expedition_heroes (player_id BIGINT NOT NULL,guid VARBINARY(128) NOT NULL,mission_serial VARBINARY(64) NOT NULL,snapshot LONGBLOB NOT NULL,PRIMARY KEY(player_id,guid),KEY(mission_serial)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
