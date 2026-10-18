-- 原VIP9礼包的下游奖励已恢复；价格和每日一份属于关闭的本地规则。
INSERT IGNORE INTO `p3_vip_shop_products`
 (`id`,`min_vip`,`cost`,`reward`,`source`,`enabled`) VALUES
 ('p3vip7_growth_local',7,800,'item_041,1',
  '本地会员成长礼包；默认关闭；800钻、每日一份为本地规则，原购买价格未恢复；原item_041资源礼包，VIP7及真实确认测试订单资格',FALSE),
 ('p3vip8_growth_local',8,900,'item_042,1',
  '本地会员成长礼包；默认关闭；900钻、每日一份为本地规则，原购买价格未恢复；原item_042→item_063宝石技能书，VIP8及真实确认测试订单资格',FALSE),
 ('p3vip9_growth_local',9,1000,'item_043,1',
  '本地会员成长礼包；默认关闭；1000钻、每日一份为本地规则，原购买价格未恢复；原item_043→item_065料理技能书，VIP9及真实确认测试订单资格',FALSE),
 ('p3vip10_growth_local',10,1100,'item_044,1',
  '本地会员成长礼包；默认关闭；1100钻、每日一份为本地规则，原购买价格未恢复；原item_044→item_064宝石技能书，VIP10及真实确认测试订单资格',FALSE);
