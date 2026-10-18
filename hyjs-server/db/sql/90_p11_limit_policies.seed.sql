-- 原活动日历未核实，默认关闭且不生成虚构周期；参数首次有效开启从原表冻结。
INSERT IGNORE INTO p11_limit_policies(id,enabled,reward_enabled,period_id,starts_at,ends_at,source) VALUES(1,0,0,'',0,0,'原LimitArenaPage/原GameConfigure；本地隔离UTC+8日额度与首次有效进入计登录分，原周期/免费购买重置待审计');
