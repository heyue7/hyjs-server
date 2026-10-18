INSERT IGNORE INTO copy_drop_rules (id,enabled,mode,source) VALUES
('ordinary',1,'relative-weights-v1','本地兼容：原DropOdds每条配置独立按数量/相对权重抽一次，包含0数量；未知原服抽样/保底算法，SpecialOdds三元保持未恢复；首通原1000确定档保留，其他首次掉落独立first规则');
INSERT IGNORE INTO copy_drop_rules (id,enabled,mode,source) VALUES
('first',1,'relative-weights-v1','本地兼容：原FirstDropOdds每条独立按数量/相对权重抽一次，单数量100/1000档按固定奖励；未知原服首通概率/保底不宣称复原，旧未冻结战斗保持已有原1000确定档');
