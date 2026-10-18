INSERT IGNORE INTO daily_copy_policies (id,enabled,room_id,max_round,win_stars,source) VALUES
('local_v1',true,'map_forest_002',40,5,'本地资源副本策略：UTC+8按原表星期开放；开始挑战扣除原表体力及一次日限，失败不返还；同日单场未结算战斗可恢复，跨日新场独立计次；胜利5星；Drop1每资源按原表数量权重独立抽取，否则使用Drop；开始冻结战斗和奖励；客户端模拟，仅校验会话、回合和剩余HP边界');
