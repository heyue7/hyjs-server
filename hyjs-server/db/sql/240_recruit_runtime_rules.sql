-- 招募全局运行覆盖；默认值由启动种子从原静态奖池/P2规则复制，运行时不改原表。
CREATE TABLE IF NOT EXISTS `recruit_runtime_rules` (
 `kind` int NOT NULL,
 `daily_limit` int NOT NULL,
 `guarantee_every` int NOT NULL,
 PRIMARY KEY (`kind`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
