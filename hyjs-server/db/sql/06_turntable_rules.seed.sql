-- 启动由 Go 首次创建，SQL 文件提供可审计初值。INSERT IGNORE 保留已有配置。
-- Unix毫秒；UTC+8：2026-10-11 00:00开始，2026-11-11 00:00结束，11-18结束排名展示。
INSERT IGNORE INTO gameplay_pools
(id,name,source,enabled,min_level,single_cost,ten_cost,single_ticket,ten_ticket,free_seconds,free_per_day,guarantee_every,coin_per_draw,start_at,end_at,reward_end_at,initial_diamond_pool,jackpot_per_million,pool_contribution_bps)
VALUES
('turntable_20261011','幸运转盘','APK价格/金币/200ppm/2%；九位奖励权重、库存、冷却、积分和排名为本地服规则',true,1,'diamond,50','diamond,488','','',86400,-1,0,10000,1791648000000,1794326400000,1794931200000,10000,200,200);
INSERT IGNORE INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock) VALUES
('turntable_20261011',0,'item_011',5,3000,false,0),
('turntable_20261011',1,'item_010',3,2500,false,0),
('turntable_20261011',2,'item_009',2,1800,false,0),
('turntable_20261011',3,'item_008',1,1200,false,0),
('turntable_20261011',4,'item_005',20,800,false,0),
('turntable_20261011',5,'item_016',1,400,false,1000),
('turntable_20261011',6,'item_001',2,200,false,500),
('turntable_20261011',7,'equip_001',1,80,false,100),
('turntable_20261011',8,'equip_005',1,20,false,50);
INSERT IGNORE INTO turntable_rules(pool_id,score_per_draw,top_prize,advance_prize,advance_places,description) VALUES
('turntable_20261011',1,'item_008,100','item_008,20',10,
'本地服规则：每24小时免费单抽一次，每抽10000金币及1积分；付费单抽50钻、十抽488钻。付费钻石2%进入共享钻石池，小数累计；大奖率0.02%，中奖获得整个钻石池。普通奖位按SQL权重和共享库存抽取。积分相同按玩家ID升序，仅真实参与者上榜。活动结束后自动结算：第1名100极品精炼石，第2至10名20极品精炼石，奖励直接入库且不重复发放。UTC+8活动2026-10-11至11-11，排名展示至11-18。');
INSERT IGNORE INTO turntable_globals(pool_id,diamond_pool,contribution_remainder,used_stock,settled,final_ranks)
VALUES ('turntable_20261011',10000,0,'{}',false,NULL);

-- 用户2026-10-17截图的新普通源。旧池/既存个人冻结/回执均保留；红武源不变。
-- 用户最终选择右上暗之幽梦兽pet_066，每次1只/库存2；右中史诗装备箱3个；权重不是截图证据。
INSERT IGNORE INTO gameplay_pools
(id,name,source,enabled,min_level,single_cost,ten_cost,single_ticket,ten_ticket,free_seconds,free_per_day,guarantee_every,coin_per_draw,start_at,end_at,reward_end_at,initial_diamond_pool,jackpot_per_million,pool_contribution_bps)
SELECT 'turntable_reference_20261017',name,'用户2026-10-17截图九奖位确认；右上暗之幽梦兽pet_066每次1只/库存2、右中史诗装备箱3个；原partial占位保护更新并保留人工改值；价格金币及大奖参数沿APK，概率/其他库存/免费/积分排名沿本地SQL',enabled,min_level,single_cost,ten_cost,single_ticket,ten_ticket,free_seconds,free_per_day,guarantee_every,coin_per_draw,start_at,end_at,reward_end_at,initial_diamond_pool,jackpot_per_million,pool_contribution_bps FROM gameplay_pools WHERE id='turntable_20261011';
INSERT IGNORE INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock)
SELECT 'turntable_reference_20261017',id,
CASE id WHEN 0 THEN 'item_340' WHEN 1 THEN 'coin' WHEN 2 THEN 'item_001' WHEN 3 THEN 'pet_066' WHEN 4 THEN 'item_027' WHEN 5 THEN 'item_138' WHEN 6 THEN 'MineItem_079' WHEN 7 THEN 'item_127' WHEN 8 THEN 'item_139' ELSE resource_id END,
CASE id WHEN 0 THEN 1 WHEN 1 THEN 50000 WHEN 2 THEN 3 WHEN 3 THEN 1 WHEN 4 THEN 3 WHEN 5 THEN 3 WHEN 6 THEN 1 WHEN 7 THEN 3 WHEN 8 THEN 3 ELSE amount END,
weight,guaranteed,CASE id WHEN 0 THEN 3 WHEN 3 THEN 2 WHEN 6 THEN 8 ELSE stock END FROM gameplay_pool_rewards WHERE pool_id='turntable_20261011';
INSERT IGNORE INTO turntable_rules(pool_id,score_per_draw,top_prize,advance_prize,advance_places,description)
SELECT 'turntable_reference_20261017',score_per_draw,top_prize,advance_prize,advance_places,CONCAT(description,CHAR(10),'2026-10-17用户最终确认：九个奖位按原图数量；右上暗之幽梦兽pet_066每次1只、库存2，右中史诗装备箱3个；荣耀随机箱库存3、7级七彩宝石库存8。此前待选说明由本次确认覆盖；概率及其余库存沿既有本地SQL，图不证明概率、每日限额或库存归属。') FROM turntable_rules WHERE pool_id='turntable_20261011';
INSERT IGNORE INTO turntable_globals(pool_id,diamond_pool,contribution_remainder,used_stock,settled,final_ranks)
SELECT id,initial_diamond_pool,0,'{}',false,NULL FROM gameplay_pools WHERE id='turntable_reference_20261017';

-- 用户最终确认暗宠和装备箱；仅修本轮尚未发布partial源的原占位，旧期、人工改值和已升级源不覆盖。
UPDATE gameplay_pool_rewards AS r JOIN gameplay_pools AS p ON p.id=r.pool_id
SET r.resource_id='pet_066',r.amount=1,r.stock=2
WHERE r.pool_id='turntable_reference_20261017' AND r.id=3 AND r.resource_id='item_008' AND r.amount=1 AND r.stock=0
AND p.source IN ('用户2026-10-17转盘截图及荣耀随机箱确认；七奖位/数量与两库存按图，其余两奖位暂沿旧普通配置；价格金币及大奖参数沿APK，概率/其他库存/免费/积分排名沿本地SQL','用户2026-10-17截图七明确奖位；两未知位及概率/其他库存沿旧普通本地规则');
UPDATE gameplay_pool_rewards AS r JOIN gameplay_pools AS p ON p.id=r.pool_id
SET r.resource_id='item_027',r.amount=3
WHERE r.pool_id='turntable_reference_20261017' AND r.id=4 AND r.resource_id='item_005' AND r.amount=20
AND p.source IN ('用户2026-10-17转盘截图及荣耀随机箱确认；七奖位/数量与两库存按图，其余两奖位暂沿旧普通配置；价格金币及大奖参数沿APK，概率/其他库存/免费/积分排名沿本地SQL','用户2026-10-17截图七明确奖位；两未知位及概率/其他库存沿旧普通本地规则');

-- 已知partial来源只升级一次，描述追加最终用户决定；保留原说明和所有人工描述。
UPDATE turntable_rules AS r JOIN gameplay_pools AS p ON p.id=r.pool_id
SET r.description=CONCAT(r.description,CHAR(10),'2026-10-17用户最终确认：九个奖位按原图数量；右上暗之幽梦兽pet_066每次1只、库存2，右中史诗装备箱3个；荣耀随机箱库存3、7级七彩宝石库存8。此前待选说明由本次确认覆盖；概率及其余库存沿既有本地SQL，图不证明概率、每日限额或库存归属。')
WHERE p.id='turntable_reference_20261017' AND p.source IN ('用户2026-10-17转盘截图及荣耀随机箱确认；七奖位/数量与两库存按图，其余两奖位暂沿旧普通配置；价格金币及大奖参数沿APK，概率/其他库存/免费/积分排名沿本地SQL','用户2026-10-17截图七明确奖位；两未知位及概率/其他库存沿旧普通本地规则');
UPDATE gameplay_pools SET source='用户2026-10-17截图九奖位确认；右上暗之幽梦兽pet_066每次1只/库存2、右中史诗装备箱3个；原partial占位保护更新并保留人工改值；价格金币及大奖参数沿APK，概率/其他库存/免费/积分排名沿本地SQL'
WHERE id='turntable_reference_20261017' AND source IN ('用户2026-10-17转盘截图及荣耀随机箱确认；七奖位/数量与两库存按图，其余两奖位暂沿旧普通配置；价格金币及大奖参数沿APK，概率/其他库存/免费/积分排名沿本地SQL','用户2026-10-17截图七明确奖位；两未知位及概率/其他库存沿旧普通本地规则');
