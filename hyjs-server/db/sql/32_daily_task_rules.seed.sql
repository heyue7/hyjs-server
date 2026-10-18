-- 本地服规则：APK 未包含原服日常任务目标、积分及单项奖励。
INSERT IGNORE INTO daily_task_rules (id,type,total,score,min_level,reward,enabled,source) VALUES
 ('local_copy',1,3,20,1,'coin,1000',true,'本地服规则；主线完成三关；每日20分1000金币'),
 ('local_recruit',5,3,20,1,'coin,1000',true,'本地服规则；真实招募三次；每日20分1000金币'),
 ('local_equip_strengthen',8,3,20,1,'coin,1000',true,'本地服规则；装备真实强化三级；每日20分1000金币'),
 ('local_treasure_strengthen',9,1,20,1,'coin,1000',true,'本地服规则；饰品真实吞噬一次；每日20分1000金币'),
 ('local_training',10,1,20,1,'coin,1000',true,'本地服规则；角色真实训练一次；每日20分1000金币'),
 ('local_combine',11,1,20,1,'coin,1000',true,'本地服规则；饰品真实合成一次；每日20分1000金币'),
 ('local_equip_refine',12,1,20,1,'coin,1000',true,'本地服规则；装备真实精炼一次；每日20分1000金币');
