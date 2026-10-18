-- 首次初值；INSERT IGNORE 保留运营修改。原服概率/库存/兑换价格未知。
-- 每红卡400/每橙卡100、十抽红卡保底、阵营1-3等间隔轮换为本地兼容规则。
INSERT IGNORE INTO gameplay_pools(id,name,source,enabled,min_level,single_cost,ten_cost,single_ticket,ten_ticket,free_seconds,free_per_day,guarantee_every,coin_per_draw,start_at,end_at,reward_end_at,initial_diamond_pool,jackpot_per_million,pool_contribution_bps) VALUES
('recruit_3','王者招募','APK价格/门槛/冷却/保底间隔；本地轮换/概率/红卡保底',true,20,'diamond,300','diamond,2800','','',172800,-1,10,10000,0,0,0,0,0,0),
('red_turntable_20261013','红武转盘（本地服）','原大转盘费用/赠币；本地红武奖位/权重/库存/周期',true,20,'diamond,50','diamond,488','','',86400,-1,0,10000,1791820800000,1794499200000,1795104000000,10000,200,200);
INSERT IGNORE INTO p2_king_rules VALUES(1,50,500,10,172800000,1791820800000,'red_turntable_20261013','APK上限/幸运/48小时；本地1-3轮换及幸运整卡选项');
INSERT IGNORE INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock) VALUES
('recruit_3',1,'hero_001',1,400,true,0),
('recruit_3',2,'hero_002',1,400,true,0),
('recruit_3',3,'hero_003',1,400,true,0),
('recruit_3',4,'hero_004',1,400,true,0),
('recruit_3',5,'hero_005',1,400,true,0),
('recruit_3',6,'hero_006',1,400,true,0),
('recruit_3',7,'hero_007',1,100,false,0),
('recruit_3',8,'hero_008',1,100,false,0),
('recruit_3',9,'hero_009',1,100,false,0),
('recruit_3',10,'hero_010',1,100,false,0),
('recruit_3',11,'hero_011',1,100,false,0),
('recruit_3',12,'hero_012',1,100,false,0),
('recruit_3',13,'hero_013',1,100,false,0),
('recruit_3',14,'hero_014',1,100,false,0),
('recruit_3',15,'hero_015',1,100,false,0),
('recruit_3',16,'hero_016',1,100,false,0),
('recruit_3',17,'hero_017',1,100,false,0),
('recruit_3',18,'hero_018',1,100,false,0),
('recruit_3',19,'hero_019',1,100,false,0),
('recruit_3',20,'hero_023',1,100,false,0),
('recruit_3',21,'hero_024',1,100,false,0),
('recruit_3',22,'hero_051',1,100,false,0),
('recruit_3',23,'hero_052',1,100,false,0),
('recruit_3',24,'hero_053',1,100,false,0),
('recruit_3',25,'hero_062',1,100,false,0),
('recruit_3',26,'hero_063',1,100,false,0),
('recruit_3',27,'hero_064',1,100,false,0),
('recruit_3',28,'hero_065',1,400,true,0),
('recruit_3',29,'hero_066',1,400,true,0),
('recruit_3',30,'hero_067',1,400,true,0),
('recruit_3',31,'hero_068',1,100,false,0),
('recruit_3',32,'hero_069',1,100,false,0),
('recruit_3',33,'hero_070',1,100,false,0),
('recruit_3',34,'hero_071',1,100,false,0),
('recruit_3',35,'hero_072',1,100,false,0),
('recruit_3',36,'hero_073',1,100,false,0),
('recruit_3',37,'hero_074',1,400,true,0),
('recruit_3',38,'hero_075',1,400,true,0),
('recruit_3',39,'hero_076',1,400,true,0),
('recruit_3',40,'hero_080',1,100,false,0),
('recruit_3',41,'hero_081',1,100,false,0),
('recruit_3',42,'hero_082',1,400,true,0);
INSERT IGNORE INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock) VALUES
('red_turntable_20261013',0,'item_005',20,3000,false,0),
('red_turntable_20261013',1,'item_008',2,2500,false,0),
('red_turntable_20261013',2,'item_001',2,1800,false,0),
('red_turntable_20261013',3,'item_020',5,1200,false,0),
('red_turntable_20261013',4,'item_021',10,800,false,0),
('red_turntable_20261013',5,'item_119',1,400,false,100),
('red_turntable_20261013',6,'item_023',1,200,false,100),
('red_turntable_20261013',7,'equip_037',1,80,false,100),
('red_turntable_20261013',8,'equip_041',1,20,false,50);
INSERT IGNORE INTO turntable_rules(pool_id,score_per_draw,top_prize,advance_prize,advance_places,description) VALUES('red_turntable_20261013',1,'item_021,100','item_021,20',10,'本地红武投放，复用原大转盘。九奖位权重3000/2500/1800/1200/800/400/200/80/20；红武037库存100、041库存50。费用50/488钻，24h免费，每抽10000金币及1积分。概率/库存/排名/日期本地配置。活动2026-10-13至11-13，展示至11-20。');
INSERT IGNORE INTO turntable_globals(pool_id,diamond_pool,contribution_remainder,used_stock,settled,final_ranks) VALUES('red_turntable_20261013',10000,0,'{}',false,NULL);
-- 原魂/精华用途和红色ID；兑换价/日限/目录本地配置。stock=0不限量。
INSERT IGNORE INTO p2_red_exchanges(id,resource_id,amount,cost,daily_limit,stock,used,min_level,enabled,source) VALUES
(1,'item_023',1,'item_020,400',1,0,0,20,true,'本地兑换目录价格及每日1次；原item_020/021用途和红色ID'),
(2,'item_119',1,'item_021,100',1,0,0,20,true,'本地兑换目录价格及每日1次；原item_020/021用途和红色ID'),
(3,'equip_037',1,'item_021,100',1,0,0,20,true,'本地兑换目录价格及每日1次；原item_020/021用途和红色ID'),
(4,'equip_038',1,'item_021,100',1,0,0,20,true,'本地兑换目录价格及每日1次；原item_020/021用途和红色ID'),
(5,'equip_039',1,'item_021,100',1,0,0,20,true,'本地兑换目录价格及每日1次；原item_020/021用途和红色ID'),
(6,'equip_040',1,'item_021,100',1,0,0,20,true,'本地兑换目录价格及每日1次；原item_020/021用途和红色ID');
-- 原随机箱候选；本地每箱单张等权。
INSERT IGNORE INTO p2_red_box_rewards(box_id,resource_id,amount,weight,source) VALUES
('item_361','hero_001',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_361','hero_002',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_361','hero_065',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_362','hero_003',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_362','hero_004',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_362','hero_066',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_363','hero_005',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_363','hero_006',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知'),
('item_363','hero_067',1,1,'原随机红箱候选ID；本地单张等权，原服概率未知');
