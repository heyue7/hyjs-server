# 85 羁绊稀疏阵位修复

本工具只输出 `Assembly-CSharp.dll`，不打包、安装或发布，也不新增运行时程序集。出战槽按原有 1～5 位置保留，玩家的阵容不会被移动或补齐。

## 证据和行为

原 80 APK SHA-256 `25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a`，其 Main SHA-256 `8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437`。合并 82/84/85 的输入 Main SHA-256 `01ab4f2c66d2b5f2bac3a74c70f90e1d51c36ac24ffa8a89e74a426e436c7dbd`。REA 静态成员证据 `ev_ff119f3083f704d4c1954f3fbc95bc86e2bd8ec32c39645ef9fc91585807ce6b` 绑定该 Main 的 SHA 和 MVID；原始与当前目标方法逐体哈希一致。原始 CIL 和定向调用者见 `analysis/fragment-box85/relations/` 下 `ui-luck.il.txt`、`selection-callers.il.txt`、`camps-model.il.txt`、`formation-identity.il.txt` 和 `replace-page.il.txt`。

`UIInstPlayerData.InitData` 为每个物理槽建立 `UIHeroFormation`，空槽的 `hero` 为 null。羁绊页面按字典 key 绘制五个位置。原 `UpdateCirclePanel` 遍历时未经空判断读取 `hero.guid`；在第 1、3 槽有人时点击第 3 槽，经过空的第 2 槽会抛出空引用，`SetCenterHero` 的刷新中断，点击闭包也尚未更新 `currIndex`。该静态和模型证据不能单独证明手机进程退出的调用堆栈。

本补丁固定原方法体并修改以下五个方法，签名不变：

- `UI.UILuck.Init(bool)` 保留当前仍有效的槽；否则选择最前的有效出战角色。全空时停止旧协程、清空旧角色详情与按钮状态，保留原空出战槽的换人入口。
- `UI.UILuck.SetInBattleHeroIcon(string, int, UnityEngine.Transform)` 的捕获值改为角色 GUID；展示图仍通过所属角色的 `heroId` 调用原 `SetHeroWithId`。
- `UI.UILuck/<SetInBattleHeroIcon>c__AnonStorey1A3.<>m__20B(string)` 以捕获 GUID 查询实际的五个槽，用同一槽更新 `currIndex`、中心角色及详情；角色已移除时按当前阵容刷新页面。
- `UI.UILuck.UpdateCirclePanel()` 在读取 GUID 前跳过空槽。
- `UI.UILuckEvent.OpenHelpPage(int)` 仅在五槽均无有效角色时结束；其余执行原模块解锁检查和替换页面流程。

原 `UI.HeroReplacePage.OnHelpBattle` 将助战候选的 GUID 与助战槽 `pos` 写入 `CSHelperOn` 21109。这个请求没有携带中心出战角色的 GUID；原请求构造和服务端资格检查保持。出战空槽点击、换人页面和返回页面的原方法体保持。

原 prefab `pages/uiluck` 的资源 SHA-256 为 `305d4e4d5203c6821fb5ba8c26334b8d98805c1186216fbc6dc769420c1d2ab5`。其 `UILuckCirclePanel` 位于 `lucks light up/circle template panel`，上方五个出战槽位于另一条 `inbattle heros queue` 分支；因此全空时隐藏圆环面板不会隐藏出战空槽入口。序列化引用及层级见 `analysis/fragment-box85/relations/luck-prefab.txt`。

## 构建

输入必须是实际待叠加的 Main 和它的完整 SHA，目录必须全新。五个目标体与 80 参考 pin 不一致时停止，由合包方检查冲突。

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_relations85/build.py \
  --input-managed /absolute/path/to/Assembly-CSharp.dll \
  --input-managed-sha256 <input-main-sha256> \
  --output-directory analysis/fragment-box85/relations/<fresh-overlay>

rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_relations85/run_tests.py \
  --baseline-managed /absolute/path/to/Assembly-CSharp.dll \
  --baseline-sha256 <input-main-sha256> \
  --managed analysis/fragment-box85/relations/<fresh-overlay>/Assembly-CSharp.dll \
  --managed-sha256 <output-main-sha256> \
  --report analysis/fragment-box85/relations/<fresh-model-report>.json
```

构建器核验方法/字段/类型 ABI、原有引用、非目标方法体和 547 个 `Battle.Engine` 方法。`OpenHelpPage` 的原解锁及打开页面指令逐条核对；出战槽入口、选人/助战请求和中心渲染方法另列为原样保留。构建器不修改输入；最终共同 Main 需以自己的 SHA 重跑并保留此前所有 helper/资源。

## 验证边界

`ModelTests.cs` 读取原输入与实际输出 CIL，以有限的 Unity、数据和网络桩解释受影响路径，另使用 dnlib 检查五个方法的栈流与 `MaxStack`。它覆盖原版空槽空引用复现、1/3 与 1/5、首槽空、连续与满槽、相同角色 ID 不同 GUID、取消后重开、目标移除、全空清理、空 GUID、无关数据异常传播，以及原 `OnHelpBattle` 的 helper GUID、槽位和 21109。桩中的发送只记录请求参数，没有调用真实网络。静态和模型通过不能替代 Unity 原机操作；合包后仍需按设备、真实登录和实际输入版本验收。

开发首轮输出 `overlay-01ab-r1` 因注入器将同一方法中的两个 `hero` 字段读取误当唯一而拒绝，未形成候选；后将目标缩至首次枚举读取，重新构建。失败不涉及输入程序集。

## 固定样本

| 输入 Main | 输出 Main | 记录 |
| --- | --- | --- |
| 原 80 `8b732cce…` | `99ee643357b83e6f91ca455bfe0b18cf043b50aa0ffb990a443553a1597e28bc` | `overlay-80-r2/manifest.json`、`model-80-r2.json` |
| 82/84/85 合并 `01ab4f2c…` | `0496bbc2f561e41e5b369227919ca55ebf67a941646eefa3293b3b9bbf8d6d43` | `overlay-01ab-r5/manifest.json`、`model-01ab-r3.json` |
| 实际 83 r3 `20ab57ad…` | `e65cd502631b61fcacf6b55a49325108b5dc38f970587865239d1188ee726199` | `overlay-83-r2/manifest.json`、`model-83-r2.json` |

三个模型各通过 37 项检查，原目标方法 pin、其余 26,941 个方法体及 547 个战斗引擎方法核验通过。01ab 的模型报告对应相同 SHA 的重建产物；最终目录使用同 SHA 且包含最新验证器记录的 r5。83 输入取自测试 APK SHA-256 `1d4b6798ffe54f2545a7d14b9109ae9bd84e7a6d66f7a17b253e213b414adaa8`，仅临时提取 Main，不修改 APK。最终共同 Main 及原机交互仍需由合包负责人核验。

补充审计 `audit-80-r2.json`、`audit-01ab-r5.json`、`audit-83-r2.json` 检查了输入的四类元数据堆原字节前缀，以及 `StandAloneSig`、`TypeSpec`、`MethodSpec` 原行；三份输出均保留原内容。构建时固定源码 SHA，若源码在构建期间变化则中止。
