-- P4 本地兼容规则，实例冻结、交易与回执在同一事务。
CREATE TABLE IF NOT EXISTS p4_commerce_states (player_id BIGINT NOT NULL,rule_id VARBINARY(64) NOT NULL,payload LONGBLOB NOT NULL,PRIMARY KEY(player_id,rule_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
