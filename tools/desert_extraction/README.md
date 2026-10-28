# 沙漠 V4 客户端补丁

本目录保存基于 1.0.78 的独立地图适配、采集状态机、原 NGUI 复用及地图候选审计工具。当前为开发中：不得直接用于正式 APK。

## 版本和地图适配

- 基础 APK SHA-256：`fe67aea7d584199119a04f10edcd8baa54207a70717e295d377ce3dd67b8affc`。
- 原 `Assembly-CSharp.dll` SHA-256：`2dfd8ca0869c24dc55e90df179ec3c545f4f91b60c44974f03460a8280d9e725`。
- 5.3 场景包 SHA-256：`2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1`。
- `build_v78_adapter.py` 基于固定 APK 生成地图和相机资源重绑，可选开启沙漠入口/恢复登录；按 `manifest.json` 逐项确认。此工具不打包、不安装。
- 已验证地图重绑 r2 的 managed SHA-256：`4eaa1d47b1d822f579e0c7bb3bfa2d9cb5b08f75a32f194be7420fc44a1a1c97`；检查时原战斗引擎 547 个方法和其余 26932 个方法体保持原样。手机行走证据来自旧 r10 沙漠空场验证，不能代表 1.0.78 或 V4 玩法验收。

## 资源采集

`FieldResources.cs` 接收服务端公共快照并调用原角色资源加载器；`GatherPreload.cs` 提前加载原采集动作和道具；`GatherAttempt.cs`、`GatherCoordinator.cs` 保留请求 UUID、1 秒截止、晚到取消、跨进程恢复和满包原奖励重领状态；`NativeGatherUI.cs` 使用原 FSM 和读条；`ProtectionRelease.cs` 需要玩家实际确认，并且不自动开始后续动作。`FieldRuntime.cs` 接入点使用原 `BoxManager`，仅在完整快照后用服务端绝对余额刷新原材料和疲劳。

`build_field_patch.py` 是独立静态预览构建器，固定输入为上述地图重绑 r2，不触碰通用 APK 打包脚本；通过后仍需由主代理合并到最终打包流程。首次预览在原方法体标记核验处失败，后续源码已修正对 token 数值的比较，尚未复跑。恢复本机性能之前禁止运行。该预览尚无战斗、NPC 入场、终端、交易和撤离完整接入，严禁视作可发布 APK。

`run_checks.py` 可在恢复性能后用原 APK 和项目 `.tools/native-batch` 运行 C#3/旧 Unity API 编译以及纯状态机竞争检查。检查不运行 Unity、Android、数据库或服务端。

## 地图

`map-candidates.json` 是 512 像素设计图经已记录变换生成的初始候选，不是发布坐标。`HyjsDesertV4PointAudit.cs`、`HyjsDesertV4Suggest.cs`、`HyjsDesertV4F09.cs`、`HyjsDesertV4FishOptions.cs` 均为 Unity 5.3 Editor 审计脚本；所有输出明确 `eligibleForRelease=false`。鱼模型和岸上站位分开核验，原鱼交互使用三维距离。候选仍须真实模型体积、同一水体、四桥怪/Boss 绕行、站位半径、原机实测及服务端配置一致性核验。

详细进度及阻塞见 `analysis/desert-extraction-v4-client/STATUS.md`。
