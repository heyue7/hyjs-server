# 试炼商店刷新客户端

基包为 1.0.80，SHA-256 `25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a`。原 APK 的 `ShopInnerPage/MemoryShop` 使用刷新栏、`SimpleGrid` 和 `ShopMemoryItem`；试炼页签沿用 `ShopInnerPage/PointShop`。商品页复用原刷新栏与商品组件、试炼积分图标以及原星数条件标签，其余页维持原组件。

按以下命令在新目录重建：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_trial_refresh82/build.py --base-apk /Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk --output-directory tools/client_trial_refresh82/overlay-80-new
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_trial_refresh82/test_callbacks.py
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_trial_refresh82/package.py --base-apk /Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk --overlay tools/client_trial_refresh82/overlay-80-new --output-directory /Users/lirui/LR/hyjs/build/trial82
```

构建验证两处目标方法以外的原方法、547 个战斗引擎方法、原辅助程序集公开接口，以及候选和 QA 的继承 ZIP 载荷、签名与版本。`test_callbacks.py` 使用原 helper 源码和边界桩，检查切页、旧回包、重复回调及原协议；它不模拟 Unity/NGUI 实际渲染。

QA 包使用独立 `.multi` 包名，HTTP 为 `127.0.0.1:18871`。TCP 由隔离服登录响应提供 `127.0.0.1:18872`，设备 reverse 和隔离服务由验收者管理。QA 包不可提升为正式版本。脚本只读取已有签名材料，不复制该材料，不启动服务、不操作设备。
