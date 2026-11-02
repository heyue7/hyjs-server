# 本机原引擎验证

用途：开发中快速排查原战斗输入、驱动、轨迹与结果，减少把每次数值诊断都排到手机。入口为项目根目录的 `tools/local_original_engine_host/`；先读其 README 和所选 case set 的固定交付。UI、资源加载、网络回包、实际消费及重登仍在对应业务批次验原机。

## 执行顺序

1. 选择已有同源 case set；核原程序集、caller、162表、wire/typed 输入及 driver/policy 的 SHA。复用已有输出时核证据；输入、环境或实现变化才执行受影响样本。
2. 按工具 `--help` 选择已固定 case set，用 `run.py --case-set <集合> --output-directory <全新目录>`。已有合格 runtime 且 DLL/表/caller 未变时，优先使用 `replay.py` 复用已编程序，参数见工具 README；需要编译时使用现有 SDK，不重复下载/restore。不覆盖旧日志或黄金，只做宿主运行时约束。
3. 先看完整 schema 比较，再定位首个差异。日志平台身份单独报告；战斗字段、事件顺序、精确数值、protobuf 字节与终末结果保持严格比较。不得静默删字段、改容差或把宿主输出标成原手机采集。
4. 新机制/模式/演员范围仍需相应原机锚点及现有 Go 层比较；已通过同源证据可复用。离线通过后把原入口、自然战斗、奖励用途和冷恢复并入下一业务批次，不为每个参数独占手机。
5. 同一宿主失败两次后先区分 Unity 边界、平台/JIT、驱动和战斗计算。做一个有界最小诊断；仍有差异就保留失败及阻塞，继续独立玩法。无需为平台栈尾部不同持续追求整条日志字节相同。

## 已验证边界

- 原 Assembly/547计算方法、DAO、原162表及原 caller 保持；只有三处非计算 Unity internalcall 元数据适配，具体限制见工具 README。禁止以工具可运行推导全部方法分支已验证。
- 原 caller 用调用栈识别 AI 回合结束。本机 CoreCLR 默认优化曾使长战停在23轮；`run.py` 只给宿主固定 `COMPlus_TieredCompilation=0`、`COMPlus_JitNoInline=1`、`COMPlus_JITMinOpts=1` 及 runtimeconfig 的 Tiered=false，编译器环境保持。不要擅自删这些设置，或把它们带入游戏服务/手机运行环境。
- 已有两 Daily 的314条历史 schema 观测相等；Trial9两短战和 Awake16两场在前一宿主配置下完整 schema4 相等。另两 Trial 长战在稳定配置下共8.147秒完成，2904/2919事件的战斗数据逐字段严格一致；完整 schema 仍因16处 `ai_notify.data.stack` 平台身份不同而 FAIL。各配置证据分别保存，不能声称同一新配置重跑了全部样本。
- 当前 `nativeCompared=false`、`authoritative=false`。本机证据可加速研发，不扩大生产 scope、不替代原13族 Go 验证或原机业务验收。

完整事实与复用命令：[初始六例及失败](../../../analysis/growth-first-20261016/p6-local-headless-p7-r1/handoff.md)、[稳定运行时两长战](../../../analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1/handoff.md)。后续版本以各固定 delivery 的 SHA 为准，保留这些历史范围。

成长诊断另有[最高觉醒003双阵营样本](../../../analysis/growth-first-20261016/p6-local-headless-growth-awake003-r1/handoff.md)：复用已编程序6.385秒，两个样本各817条共同历史观测严格一致；完整 schema 及历史未重采技能图、新AI栈/终末字段仍有明确缺口。演员来自实际最高培养存档，原pose变化及旧诊断optRecord均有来源说明；不能把该离线输入当成当前账号或原网络End。
