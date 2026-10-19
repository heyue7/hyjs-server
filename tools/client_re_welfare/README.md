# Re 招财猫与连续消费原页面桥

四个原方法有固定 raw SHA pin：猫的领奖回调与状态展示；连续消费的查询回调与未达标前往按钮。输入可组合其他无关覆盖层，工具会逐字检查其余全部原方法及 547 个战斗引擎方法保持，并保留原元数据、程序集身份和之前的 helper 引用。

- 招财猫沿原请求、奖励动画、奖励弹窗与原 `AddSimpleItem`。成功回调先按上一份原 Info 的实际钻成本调用原 `SubDiamond`，完整返钻由原回调加入缓存，与服务端净额一致。同页同一成功回包不重复加入。
- 五次成本/完整返钻由服务端冻结 Tab ID `fortuneCat_re_current~成本,返钻...` 显示元数据提供。原猫请求没有活动ID字段，领奖资格、实际扣款与发奖仍在服务端判断。桥只在 `fortuneCat_re_` 页面触发。
- 原剩余 `/3`、过期按钮/冷却改为真实5次；显示当前成本、完整返钻与净增。原 NGUI 字体与页面组件保持；生成按钮文字时优先保留原 `bitmapFont`，不随后覆盖 `trueTypeFont`。
- 连续消费只对 `continuedRecharge_re_` 改为实际消费文案和“去消费”按钮，前往原 `ShopPage`。达标后仍沿原日档/终奖请求、资源清单和领奖刷新流程。

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 .analysis-venv/bin/python tools/client_re_welfare/build.py \
  --input-managed analysis/client-reward-redpoint-20261019/candidate-r1/Assembly-CSharp.dll \
  --output-directory analysis/re-operations-welfare-20261019/client-candidate-r3
```

输出仅包含托管文件和静态资格，不创建或安装 APK。打包时保留其他全部 helper，并增加 `SoulFlowerReWelfare.dll`。


## 界面反馈修复（2026-10-19）

`--helper-only` 重新编译已有四个桥接方法使用的 helper，输入必须已经引用 SoulFlowerReWelfare；输出 main 逐字节不变。用于在当前组合 main 上修 UI，避免重复注入。

- 猫消耗/返还说明写入原 Root 下可见的兄弟 UILabel；原 solgon/solgon2 是金币池字图的父节点，隐藏时其子数值也会隐藏。采用原 UIFont、原人物/背景/按钮和固定左侧350宽区域。顶部两旧label同时隐藏，在原Root中的350宽label显示余次；原btnCharge根UISprite（UI_button_zhaomu_02、176×64）明确启用并绑定button.target，仅隐藏旧探索字图；中央展示本期冻结的真实5档报价。
- 连续消费的顶部恢复原 prefab 306×40边界及 ShrinkContent，显示实际消费/门槛；累计天数文案沿原占位长度改为消费达标累计，保留原7天数值与领取分支。
- 当前候选和原机证据分别记录在 `analysis/play-feedback-20261019/welfare`；静态编译通过不代表新候选已经原机验证。
