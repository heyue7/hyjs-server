# 1.1.4 采集恢复 helper 构建记录

此构建只生成可交给 `package_native.py` 的 managed helper 与清单。它保留 1.1.4 的离图、位置同步和沙漠首领源码，并加入 `dc030d6c4de1b7c06815b1090f267a4642d604bd` 的采集状态恢复。它不生成 APK，也不操作设备或服务器。

运行环境使用项目现有 `.analysis-venv` 和 `.tools/native-batch`。在包含本脚本与上述采集提交的独立工作树根目录运行，输出目录须为本工作树内不存在的新目录：

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/desert_extraction/build_gather_status_overlay.py \
  --apk '/Users/lirui/LR/hyjs/deliveries/client-update-1.1.4/黑衣剑士灵魂之花v1.1.4-离图与沙漠首领候选.apk' \
  --runtime /Users/lirui/LR/hyjs/.tools/native-batch \
  --output build/gather-status-overlay-for-package-20261101
```

脚本将 1.1.4 APK、31 个 helper 生产源码、源文件清单、资源、构建器及精确允许的 ABI 增量钉定。它先逐字节重建已安装的 helper，之后构建候选，并在 `manifest.json` 中记录文件、引用和哈希。产物使用的旧 helper SHA-256 为 `3080d87917cf578f073f303e9c3bb33fbfb99bae8cb964b344890bf37c77b68b`，两次独立重复构建的候选 SHA-256 均为 `69baddae8f6d197cf2c50edade43435447ba08f8f3483bdf7b9fd3b194b270f4`。相对 1.1.4，生产源码仅 `GatherAttempt.cs` 和 `GatherCoordinator.cs` 改变。已有 435 项公开 ABI、8 项程序集引用及采集状态编号 0–6 保持，允许新增 3 个采集方法与 `CheckingClaim=7`。输出清单包含 `package_native.py` 的 source pins、payload、基础 APK 和程序集哈希字段；合包前需在执行合包的同一工作树重新生成。

本次模型与静态验证使用以下命令；前三条执行真实源码与隔离桩，第四条还检查首领地图调用和 helper 编译。1.1.2 与 1.1.4 的九个 helper 编译引用逐项哈希一致。

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 DOTNET_PROCESSOR_COUNT=2 DOTNET_CLI_TELEMETRY_OPTOUT=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/desert_extraction/run_gather_status_checks.py --runtime /Users/lirui/LR/hyjs/.tools/native-batch --apk '/Users/lirui/LR/hyjs/deliveries/client-update-1.1.4/黑衣剑士灵魂之花v1.1.4-离图与沙漠首领候选.apk'
rtk proxy env PYTHONDONTWRITEBYTECODE=1 DOTNET_PROCESSOR_COUNT=2 DOTNET_CLI_TELEMETRY_OPTOUT=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/desert_extraction/run_leave_recovery_checks.py --runtime /Users/lirui/LR/hyjs/.tools/native-batch --apk '/Users/lirui/LR/hyjs/deliveries/client-update-1.1.2/黑衣剑士灵魂之花v1.1.2-修复候选.apk'
rtk proxy env PYTHONDONTWRITEBYTECODE=1 DOTNET_PROCESSOR_COUNT=2 DOTNET_CLI_TELEMETRY_OPTOUT=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/desert_extraction/run_position_sync_checks.py --runtime /Users/lirui/LR/hyjs/.tools/native-batch
rtk proxy env PYTHONDONTWRITEBYTECODE=1 DOTNET_PROCESSOR_COUNT=2 DOTNET_CLI_TELEMETRY_OPTOUT=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/desert_extraction/run_field_boss_countdown_checks.py --runtime /Users/lirui/LR/hyjs/.tools/native-batch --apk '/Users/lirui/LR/hyjs/deliveries/client-update-1.1.2/黑衣剑士灵魂之花v1.1.2-修复候选.apk'
```

采集模型和协调器/日志存储交错测试通过，离图 244 项、位置同步 168 项、首领和地图 80 项断言通过。合包器的 overlay 入口、基础程序集、DEX、全部 source pins 与 helper payload 已在不打包的情况下校验。真实 Unity 运行、手机网络交互与原机验收仍待后续完成。

旧版客户端无法识别新增的 `CheckingClaim=7` 记录；若回退时该记录仍在，旧版会保守阻塞，需升级回支持该状态的客户端继续核验。
