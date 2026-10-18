INSERT IGNORE INTO skill_specialty_rules (id, enabled, initial_types, unlock_policy, source)
VALUES ('default', TRUE, '1,2,3', 'table_pair_strength', '本地明确规则：首次达到原75级和品质门槛，类型1/2/3免费从等级1强化1开始，4/5锁定；canOpen解释为所在类型与指定类型同时达到强化门槛后开启目标。原表费用、属性和上限沿用，初始值及解锁解释缺少原服务端实现证据。');
