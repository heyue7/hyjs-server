# 会员礼包原界面空态

用户明确：先修展示与后台，七商品继续停用。此工具不请求接口、不改商品、不模拟奖励，只呈现成功的原shopType2空列表。

原main输入r7 SHA e4e577fe401920ef3f17ced0f48d761d74437bca42d2e4fd2b6db15d4f230689，方法唯一pin：`UI.TabVipShop.OnMsgCallback` raw SHA76351e9d2269e846d1b20a644a9eb2b215d9f0d9b794a8bbaee941ad59360685。REA parent `ev_884be5bbd6a6cd3b33d41d5e2c91579112656b2eb7a3390fdfb8536971297996`；原IL在recruit-shop/vip-ui-il.txt。

后置调用`VipShopEmpty.Received(tab, packet, success)`。仅成功SCShopProductList、shopType2且count0时显示“暂无开放的会员礼包”。字体从原shopItemPrefab UILabel取，保原游戏中文字体；出现任意真实商品后隐藏。消息放原scrollView裁切区域中心；无按钮、无网络或购买副作用。加载失败沿原错误通道处理。

`build.py`支持任意前驱main（该唯一目标须仍匹配）。可叠世界r4、福利、公告等其他overlay；须同时打入`SoulFlowerVipEmpty.dll`。保持547原引擎与其他原方法raw body、ABI/token，原始所有方法/字段/类型数量不变，仅增加外部helper引用。工具只编译/生成overlay，不打包/安装。

```
rtk proxy .analysis-venv/bin/python tools/client_vip_empty/build.py \
 --base-apk apks/build/hyjs-dialog-final-r7.apk \
 --base-sha256 eba29af9e52db8b2af751f568ef51f1a98e52611457c16720d2ce4ecbd0389f7 \
 --input-managed /absolute/path/to/predecessor/Assembly-CSharp.dll \
 --output-directory /absolute/path/to/new-output
```

当前vip-empty-r1叠世界r4已静态通过，原机字体/位置仍待root验证。后台shop-workspace复用原7条关闭配置编辑，新增停用空态、可读状态、真实每日1份/付款资格展示；未改Enabled或价格。


## 同价旧价行修复（2026-10-27）

同一后置hook在成功shopType2回包完成原生建卡后，按原排序后的列表和grid索引对应商品；单项price/discount的资源id及num相同则隐藏整条PriceBefore，保留原PriceAfter。不同价格恢复PriceBefore，字体、坐标、素材和购买处理维持原实现。计数或层级异常时跳过；空态逻辑仍保留。兼容服务端为避免原客户端读取空Discount越界而返回同价的协议。

原证据：`apks/original/10056262.apk` 中 `assets/bin/Data/aa6d7e3e12e66c94cae7d12beba87580`（ResourceManager的shopinnerpage/giftshop）：ShopItem Transform58下PriceBefore Transform49（GameObject17），包含Value Transform36、logo42、Diamond44及Sprite41（划线）；PriceAfter Transform67为独立兄弟节点。原 `UI.ShopVipItem.SetShopVipItem` IL 0xce至0xec写Price[0].num到labPriceBefore，0xf2至0x110写Discount[0].num到labPriceAfter。原TabVipShop先对回包Sort，再按顺序AddChild/SetShopVipItem。因此不依赖名称扫描全局物体、不改变购买数据或价格字段。

静态验证：使用本机 .NET SDK 8.0.426 Roslyn，`/langversion:3 /nostdlib+` 对原Managed引用编译helper通过（2026-10-27）；未运行Unity、未重新打包、未操作手机。原Injector和pins未改，新增行为全部位于helper。

集成时重编`SoulFlowerVipEmpty.dll`并将其替换进最终APK；已含空态hook的前驱Assembly-CSharp无需再次执行Injector（其pin本来也会拒绝重复注入）。共享打包入口应采用新helper，不复用旧二进制缓存。未含hook的前驱可用原build.py生成overlay。原机检查：同价1888应仅剩真实1888及货币图标；真实折扣应仍显示旧价划线和新价；购买后列表刷新、切换页再进入、空列表与重新开放商品均需检查。当前不声称原机通过。

## VIP7 直接附赠书的完整预览（2026-10-27）

`VipGiftPreview.cs` 只处理第一项为 `item_041` 且后续直接奖励含 `item_063` 的商品。固定原五项箱内容须与客户端表的 result 完全一致；按箱数量展开一次，再追加回包里的直接奖励。沿用原 `ShopGiftPreview` 的 `type=3&activityGetGiftId=id,num;...` 分支，继续使用原图标、名称、数量和布局。单项、其他礼包、无书、空值、非法ID、数量溢出、表偏离或异常返回原点击入口；不改奖励列表、物品表、购买或发奖。

原 `Assembly-CSharp.dll` 的只读 dnfile/dncil 证据：`UI.ShopVipItem.OnIconClick` IL 0x001b–0x003b 仅传 `item[0].id` 给 type1；`UI.ShopGiftPreview.SetItem` IL 0x005d–0x006b 读 `Dict.Blo.DictItemBlo.GetTableItemUse(itemId).result`。`DoOpen` IL 0x00ac–0x00ea 的 type3 从 `activityGetGiftId` 取字符串，经 `ParseParams`（分号/逗号拆分、Int32.TryParse）交给 public `void SetItemBySever(List<GiftItem>)`。`UI.PageManager.OpenPage` 的原签名为 instance `void OpenPage(string,string)`（签名 blob `2002010e0e`）。原VIP7表见 `analysis/tables/TableItemUse.tsv:13`。

专用 `build_preview.py` 要求已含空态hook的指定前驱，严格pin原 `OnIconClick` 和已存在 `OnMsgCallback`；`PreviewInjector.cs` 只前置 `TryOpen(shopItemInfo)`，false分支回到原第一条指令。私有字段在原方法内读取；不使用反射、不改可见性。原空态Injector不再运行。校验仅一个原方法变化、原回退指令完全保留、547引擎不变及原ABI/token不变；已注入预览的输入在创建输出前拒绝。

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_vip_empty/build_preview.py \
  --base-apk build/vip-ui-device-test-20261027.apk \
  --base-sha256 0381a28b7ade9788462925483a603de3a98c25a274c4871e1b049577c9468eae \
  --runtime-root /Users/lirui/LR/hyjs/.tools/native-batch \
  --output-directory build/vip7-combined-preview-20261027-final
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_vip_empty/test_preview.py
```

将输出 `Assembly-CSharp.dll` 和 `SoulFlowerVipEmpty.dll` 分别覆盖 APK 的同名 `assets/bin/Data/Managed/` 条目。helper同时编译当前空态/同价行逻辑。首次编译暴露缺少 Dict 命名空间，补齐后原Managed引用C#3编译通过；行为测试覆盖精确五项+书、数量、额外直接奖励、输入不变及14种回退；重复注入拒绝通过。暂存编译目录自动回收；未打包、安装或操作手机。主代理须检查VIP7预览六项、图标/名称/滑动、其他礼包回退、购买刷新及到账无重复，最终运行验收前保留未验标记。
