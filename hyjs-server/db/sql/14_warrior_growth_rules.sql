-- 费用、开放等级和属性上限沿用APK；本表只配置缺失的服务器随机规则。
CREATE TABLE IF NOT EXISTS warrior_growth_rules (
 id VARCHAR(32) NOT NULL PRIMARY KEY,
 enabled BOOLEAN NOT NULL,
 distribution TEXT NOT NULL,
 critical_multiplier BIGINT NOT NULL,
 reset_seconds BIGINT NOT NULL,
 source TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
