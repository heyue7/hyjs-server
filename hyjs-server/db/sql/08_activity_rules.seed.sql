-- 周期为本地服规则；VIP2和1000钻来自APK客户端。UTC+8固定日期。
INSERT IGNORE INTO activity_rules (id,enabled,min_vip,diamond_cost,start_at,end_at,reward_end_at,source) VALUES ('open_server_fund',1,2,1000,1791648000000,1794326400000,1794931200000,'APK VIP2/1000钻；周期为本地服SQL配置，未购买角色可领全民福利');
INSERT IGNORE INTO activity_rules (id,enabled,min_vip,diamond_cost,start_at,end_at,reward_end_at,source) VALUES ('totalLogin_local_20261011',0,0,0,1791648000000,1794326400000,1794931200000,'本地服SQL规则：UTC+8成功角色登录日期累计，每日一次、缺席不补。1/2/3/5/7天奖励，与月签/豪华/限时签到独立');
