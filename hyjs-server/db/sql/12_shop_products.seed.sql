-- 仅初始种子；修改现有商品不会被重启覆盖。原价沿用APK，目录/数量/限购为本地服规则。
INSERT IGNORE INTO shop_products(id,source,enabled,resource_id,amount,cost,daily_limit,min_level,min_vip,priority) VALUES
('itemShopId_8','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_006',1,'diamond,20',20,1,0,1),
('itemShopId_9','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_007',5,'diamond,30',20,1,0,2),
('itemShopId_10','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_008',1,'diamond,50',20,1,0,3),
('itemShopId_11','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_012',5,'diamond,15',20,1,0,4),
('itemShopId_13','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_005',1,'diamond,20',20,1,0,5),
('itemShopId_14','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_022',1,'diamond,5',20,1,0,6),
('itemShopId_16','APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次',FALSE,'item_024',1,'diamond,88',20,1,0,7);

-- 原悬赏缺通缉令时打开原BuyItemPage；原价200钻、1..10档，新增目录映射具名记录。
INSERT IGNORE INTO shop_products(id,source,enabled,resource_id,amount,cost,daily_limit,min_level,min_vip,priority) VALUES
('itemShopId_12','wanted-order-original-fixed-slot-v1：原WantedMission.OnAddHitTimes缺004时按资源ID进入BuyItemPage；原ItemUse376固定004→wantedAttTimes1，ItemShopPrice12原固定200钻/1..10档。追加本地目录映射slot12→004一份、每日10，不改变原价；原私服完整商品目录映射未知。付费道具次数沿原AddSimpleItem直接增加，不按自然恢复上限10截断。',TRUE,'item_004',1,'diamond,200',10,1,0,8);
