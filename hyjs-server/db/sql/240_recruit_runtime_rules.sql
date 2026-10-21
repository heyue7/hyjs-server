-- 招募全局运行覆盖；默认值由启动种子从原静态奖池/P2规则复制，运行时不改原表。
CREATE TABLE IF NOT EXISTS `recruit_runtime_rules` (
 `kind` int NOT NULL,
 `daily_limit` int NOT NULL,
 `guarantee_every` int NOT NULL,
 `luck_exchange_cost` int NOT NULL DEFAULT 0 COMMENT '王者红卡兑换幸运值，0兼容旧值500',
 PRIMARY KEY (`kind`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
