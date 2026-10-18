-- P4 独立实例、冻结规则和回执；仅同事务保存成功业务。
CREATE TABLE IF NOT EXISTS p4_welfare_states (player_id BIGINT NOT NULL,rule_id VARBINARY(64) NOT NULL,snapshot LONGBLOB NOT NULL,payload LONGBLOB NOT NULL,PRIMARY KEY(player_id,rule_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
