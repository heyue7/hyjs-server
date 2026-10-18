-- 原小时配置全量冻结，默认关闭免战石新消费；既有到期仍可查询。
INSERT IGNORE INTO p11_plunder_policies(id,avoid_enabled,item_hours_json,item_hours_hash,source) VALUES(1,0,'{"item_014":8,"item_015":1}','331d868673330932b158cabc66e068d720c0fef8db8997ddeca45932828e1fbd','原TableItem category6 exp=8/1小时，原剩余毫秒；主动攻击清除与叠加上限待审计');
