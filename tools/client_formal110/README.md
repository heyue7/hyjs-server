# HYJS 1.1.0 正式候选包

该入口从已交付的 1.0.91 APK 构造独立候选：API 地址从本机 `ssh -G ubuntu@heyue-cn-1` 读取，HTTP 端口固定为 `28861`，Android 版本固定为 `1.1.0 / 2026110101`。TCP 地址仍由游戏服务器的登录响应提供。入口只解析 SSH 配置，不连接服务器，也不安装或提升 APK。

从本工作树运行；在当前进程环境中安全设置 `HYJS_FORMAL110_KEYSTORE_PASSWORD` 后执行：

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_formal110/build.py
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_formal110/verify.py
```

签名密码通过进程环境交给签名工具，不进入命令参数、仓库或证据。输入 APK、证书摘要、签名文件、运行时、输出位置和共享构建锁由脚本固定。输出或证据已存在时拒绝覆盖；失败记录位于 `analysis/formal110/failure.json`，重试前须先检查并保留所需证据。

输出为 `deliveries/formal110/黑衣剑士灵魂之花v1.1.0.apk`、`analysis/formal110/manifest.json` 和独立复核结果 `analysis/formal110/independent-verification.json`。脚本验证四个托管方法、547 个原引擎方法、其余托管方法、钱包配置、清单语义、包名与启动入口、签名、对齐以及全部其他 APK 条目和压缩数据。独立复核再次检查新地址、全部 DEX 与兼容资源 pins。每次构建在主工作区持有 `build/.client-build.lock`，并检查磁盘余量与现有 current/rollback APK 的摘要。

这些静态检查不证明新服务器可用；交付前仍需在授权的设备和服务器上验证登录、服务器返回的 TCP 地址、钱包和重连。
