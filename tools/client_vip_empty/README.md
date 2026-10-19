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
