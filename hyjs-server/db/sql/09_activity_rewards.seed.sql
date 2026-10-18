-- 奖励档位直接来源APK TableOpenServerFund/TableEveryoneBenefit；只插入缺失行。
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','openServerFund',10,'diamond,500');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','openServerFund',25,'diamond,1000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','openServerFund',35,'diamond,1500');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','openServerFund',45,'diamond,2000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','openServerFund',55,'diamond,2500');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','openServerFund',60,'diamond,2500');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',10,'hero_exp_004,1');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',30,'coin,100000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',50,'coin,200000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',100,'item_001,10');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',200,'item_010,100');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',300,'item_012,100');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',500,'item_006,100');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',1000,'equip_034,1');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',1500,'diamond,1500');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',2000,'diamond,2000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',3000,'diamond,3000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('open_server_fund','everyoneBenefit',5000,'diamond,5000');
-- 以下totalLogin奖档为本地服SQL配置。
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('totalLogin_local_20261011','totalLogin',1,'coin,10000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('totalLogin_local_20261011','totalLogin',2,'item_001,1');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('totalLogin_local_20261011','totalLogin',3,'diamond,50');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('totalLogin_local_20261011','totalLogin',5,'coin,30000');
INSERT IGNORE INTO activity_rewards (activity_id,reward_type,threshold,reward) VALUES ('totalLogin_local_20261011','totalLogin',7,'diamond,100');
