-- P4 本地兼容规则，实例冻结、交易与回执在同一事务。
CREATE TABLE IF NOT EXISTS p4_commerce_stocks (rule_id VARBINARY(64) NOT NULL,product_id VARBINARY(64) NOT NULL,used INT NOT NULL,PRIMARY KEY(rule_id,product_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
