# 客户端游玩修复覆盖层

此工具在固定的已验 APK 上生成独立 DLL，不构建、签名、安装 APK，也不连接服务器。

- 原充值回调：原订单/商品匹配及服务端 `PAY_STATUS_SUCCESS` 分支清除 pending 后增加一次“购买成功”，保留原失败、等待和刷新路径。
- 原领取引导：仅第 59 步、`guide_copy_reward`、首章第一个宝箱已领取时继续原下一步，保留未领取和其他引导。
- 初始教学跳过：在原 GuideManager 的 Awake 附加一个独立 NGUI 控件，右上角显示“跳过 >>”。使用原字体和 NGUI 摄像机布局。点击调用当前 API origin 的 `/game-guide/skip`，收到服务端成功提交后，停止当前初始教学协程、还原按钮 delegate、清理遮罩，以原最后一个初始教学行续接以后等级指引。失败/超时保留原教学，可幂等重试。
- 下阵桥：允许主角下阵，阵容至少保留一名角色；收到原 21049 确认和 20111 权威列表后解绑已移除阵位的角色、装备、饰品、精灵本地 owner，库存保留，重新打开首个剩余角色。与服务端整槽解绑行为配套。

```sh
rtk proxy .analysis-venv/bin/python tools/client_playability/build.py \
  --base-apk apks/build/hyjs-local.apk \
  --output-directory analysis/client-playability-new-overlay
```

产物：`Assembly-CSharp.dll`、`LocalPlayability.dll`、`LocalFormationUI.dll` 和 manifest。原主 DLL 只增加 Awake 启动调用、领取步骤判定与成功提示；编译器保留原 token、方法/字段 ABI，验证全部其他方法 raw header/IL/EH 字节一致以及 547 个原 Engine 方法不变。独立桥依赖旧 Unity/原游戏 DLL 编译，不执行外来 DLL。

客户端 API origin 从现有 `ServerConfig.GetServerUrl()` 派生，源码不写入服务器地址或凭据。新 HTTP handler 复用原角色 ID/会话和事务锁，跳过选择存入原 profile KV，不新增数据库表；原协议数值 12 只结束初始教学段，不发奖励、改主线、提升等级或解锁后续功能。

可选的 `verify_representatives.py` 用显式 stubs 执行生成的原回调和领取步骤分支，检查失败/重复确认不误报、未领不跳步。该结果属于 IL 控制流验证，不代表 Unity、手机或真实交易验证。
