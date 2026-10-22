# 全部原表引导跳过

`build.py` 接收固定正式48 APK（SHA256 `57bf8aa466db9175c4eb849026383a59d89080f9c54b63ac4fcc5b81c32ce799`），重新编译同款 NGUI 控件并在原登录 KV 回调、引导资格方法接入逐角色跳过记录。原方法签名、token、类型库存及547个引擎方法保持原样。

构建输入为 `--base-apk`、存有 `.tools/native-batch` 的 `--tool-root` 和全新 `--output-directory`。输出 manifest 固定源码及制品摘要；经现有客户端更新层构建后，使用 `tools/client_update/package.py --guide-overlay` 组合签名。正式 APK 使用原生产入口；手机隔离 QA 另作仅回环入口替换，禁止作为正式交付。

服务端 `/game-guide/skip` 可带原表 `guideId`；不带时兼容旧初始跳过。会话、等级及原表校验后保存 `client.guideSkipped.<guideId>`，确认成功才清当前遮罩。跳过不执行被提示的业务操作、不加资源或奖励。
