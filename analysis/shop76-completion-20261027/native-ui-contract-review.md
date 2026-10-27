# 原客户端会员与折扣协议核查

APK 1.0.76/build2026102705，SHA256 5740a0f495f58de23049a16d5ba10aec79ebec5911d8ecf92bb197ccfa85654f。设备安装文件同SHA。

- UIActivityVipBenefits.UpdateDailyTab 遍历daily，按当前vip和vip+1各取一条；缺下一档时将当前档复制到下一档预览。原服务仅给当前档，真机会员10重复11000金币。新增下一档只读预览，当前领取状态、同一账本和规则保持；vip12无虚构13档。
- VipBenefitWeeklyReward.SetReward 调用ParseDiscount，0<折数<10才显示折扣角标。原ParseDiscount先Int32.TryParse，再除10。原weekly discount="1"显示0.1折；修为"100"无折扣角标。P4 ActivityShopItem同字段改整数百分比（45显示4.5折），实际价格计算不变。
- 同状态前图：activity-pages/member-benefit.png、member-weekly.png；发布后同状态后图：member-daily-after-release.png、member-weekly-after-release.png，实机当前10/下一档11及无假折扣已通过。原547战斗方法保持。
- 正式服只执行正常购买和原入口查看；未测试发奖、未发送公告/邮件/奖励/补偿。正常VIP购买及开箱来自上一续接已授权任务，同一非保护角色14。
