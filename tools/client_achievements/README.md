# 成就与原碎片控件小桥

固定输入为 `50c9781ac6bf61984a74cd4e7517519792bdb73a0778abcce48d8815f747c35e` 的当前程序集。工具仅生成托管文件，不打包、安装或操作账号。

- 成就 37–40 使用服务端累计登录、图鉴种数、不同主线关数、上阵人数的真实进度。
- 成就 41 从原 `AchievementInfo.id` 的 `canonicalID~loginDays~level~stages` 读取角色冻结阈值，进度为达成条件数 / 3。领奖仍回传整个原 ID，资格与奖励由服务端处理。
- 原成就 1–36、原按钮 0 可领 / 1 前往 / 2 已领、原领奖和 PageManager 导航流程保留。新类型只设置原页面字段并覆盖显示文本。
- 原精灵碎片缓存分支错误地调用 HeroIcon；改为对应 PetIcon，保留原碎片数量和合成门槛判断。
- 装备/角色碎片的原逐物品红点已有数量判断；背包分类与领奖中心外部红点由原 SCRedPoint → RedPointTip → Town/Bag 订阅链更新，相关服务端点位由主代理补齐。

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 .analysis-venv/bin/python tools/client_achievements/build.py \
  --input-managed analysis/client-achievements-20261019/input-Assembly-CSharp.dll \
  --output-directory analysis/client-achievements-20261019/candidate-r1
```

`target-pins.json` 锁定 3 个方法原始字节；构建检查所有其他原方法字节、547 个战斗引擎方法、原元数据 token/签名/flags、旧配置编译器引用兼容。旧 APK 里的其他 helper 文件必须保留。产物标记静态验证，实际原机由主代理合批进行。
