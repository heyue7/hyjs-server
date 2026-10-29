# 84 客户端扫荡节奏 overlay

输出仅包含 `Assembly-CSharp.dll`，不打包 APK，不更改服务端或资源。参考原始 1.0.80 APK SHA-256 为 `25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a`，其程序集 SHA-256 为 `8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437`。

| 原方法和定位 | 修改 | 保持内容 |
| --- | --- | --- |
| `<SweepNext>c__IteratorF4::MoveNext`，原 IL_002e | `WaitForSeconds(0.5)` 改为 `0.05` | 原计数、确认按钮与下一次 `AskFight` |
| `<_StartSweep>c__Iterator141::MoveNext`，原 IL_0075–0097 | 保留 `titleAlpha.duration + bodyScale.duration` 的原指令，在构造等待前插入 `pop; ldc.r4 0.05` | 原计数、`needStop`、`CanContinue` 和 `OnSweepCopyAuto` |
| `UI.PVPSweepPage::CreateItem`，原 IL_007c | 列表 `TweenPosition.Begin` 参数 `0.2` 改为 `0` | 原坐标计算、素材和每行入场 Tween |
| `UI.UICopySweep::Refresh`，原 IL_0124 | 列表 `MoveUpPosition` 参数 `0.2` 改为 `0` | 原坐标计算、奖励解析和每行入场 Tween |

原 `TweenPosition.Begin` 在非正 duration 分支立即执行 `Sample(1, true)` 并禁用 Tween，因此每次先落到完整目标位置，再计算下次滚动目标。两处 UI 行动画依旧使用原组件与时长。所有原目标指令及其分支和令牌均保留，三个指定常量及副本等待前的两条插入指令除外；分支相对字节位移因插入而变化。其余原方法体连同异常区域逐字节比较。

原 prefab `pages/uicopysweep` 的 titleAlpha duration 为 0.20 秒，bodyScale duration 为 0.25 秒，配置下等待为 0.45 秒；修改后为 0.05 秒，每次少 0.40 秒。竞技每次成功回包后的等待从 0.50 秒改为 0.05 秒，少 0.45 秒；包含最后一次显示确认前的等待。数值为客户端静态等待配置，真实经过时间还受帧调度、网络、服务端和原动画影响。

全程序集扫描表明：竞技 `SweepNext` 仅由 `AskFightCallback` 调用，且在成功分支更新结果后启动；副本 `_StartSweep` 仅由自动扫荡回包调用，且在 `Accept`、结果更新及 `CanStartNext` 后启动。竞技仍发送原 `23101`，副本仍发送原 `54009`，每次按原回包推进。副本现有 `BeginSend`、Stop/Close、页面代次和旧回包处理原样保留。原竞技链没有新增角色或会话防旧回包机制；副本旧成功回包按原 helper 使用当前 `StaticData.playerData` 入账，未绑定角色或会话身份。

## 合并时重建

等待其他补丁合入后，使用当时实际的程序集和 SHA 重跑；不要直接将仅含本补丁的 80 基线 DLL 覆盖合并程序集。此工具验证传入基包和 managed SHA，并在输入程序集上按语义定位四处修改，所有其他方法体逐字节比较。被其他补丁修改过的目标方法须继续满足对应定位和控制流保护，发生冲突时构建失败。

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_sweep84/build.py \
  --base-apk /Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk \
  --base-sha256 25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a \
  --input-managed <最终合并后的Assembly-CSharp.dll> \
  --input-managed-sha256 <该文件的实际SHA256> \
  --output-directory analysis/feedback84/sweep-client/<全新输出目录>

rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_sweep84/run_tests.py \
  --managed analysis/feedback84/sweep-client/<全新输出目录>/Assembly-CSharp.dll \
  --expected-managed-sha256 <输出DLL实际SHA256> \
  --report analysis/feedback84/sweep-client/<全新测试报告>.json
```

`--base-apk` 也可传实际合包基包，`--base-sha256` 必须匹配该文件；`--input-managed` 和其 SHA 必须是前置补丁合并完成的输入。基包并不自动提供外部辅助 DLL，合包方须保留其他 overlay 的所有辅助 DLL 与资源。本工具默认使用 `/Users/lirui/LR/hyjs/.tools/native-batch` 下的 dotnet/dnlib，可用 `--tool-root` 指定相应工具根。

## 证据与边界

- `analysis/feedback84/sweep-client/overlay-80/manifest.json`：基线输出 SHA `92f2529ae20fb32e3ca3a20e6d83eb021324e1700a4ce59a5b01ba5452aa2d5d`；其他 26,942 个方法体和 547 个战斗引擎方法一致。
- `analysis/feedback84/sweep-client/overlay-after-trial82/manifest.json`：真实 82 试炼前置程序集 `ae454815…` 上输出 `9177fa8d…`，两处 `UI.TabPointShop` hook 与输入逐字节一致，见 `trial82-hooks.json`。
- `original-il.txt`、`patched-il.txt`、`guard-il.txt`、`coroutine-callers.txt`、`prefab-bytes.txt` 保留原程序集、前后 IL、调用方和 prefab 引用/时长证据。对应只读工具为 `dump_il.py`、`find_calls.py`、`prefab_probe.py`。
- 两份 `dotnet-tests-*.json` 的 .NET 测试读取实际产物的协程 IL，使用 Unity 调用桩解释执行；已有 StaminaSweep 源码另用桩编译，覆盖次数上限、停止、关闭、旧代次成功/失败和重复回包。这些结果不等于 Unity 原机执行。
- Wanted 与试炼的最终合并输入尚未冻结；最终输入必须重跑 overlay 和测试，再由合包方核验实际输入/输出 SHA。原机列表滚动与停止操作、快速/慢速网络、连续领取及成本/奖励恢复均待实测。
