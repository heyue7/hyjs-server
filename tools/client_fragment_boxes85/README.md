# 碎片选择箱批量使用：客户端 85

本目录在给定的已合并 `Assembly-CSharp.dll` 上追加窄补丁，输出主程序集和独立 helper。玩家在原背包详情使用原有 ±1、±10 选择数量，再进入原碎片选择页。确认时沿用 21089 `CSItemUse`、原 `getItemIndex` 和原成功回包处理；每次数量限制在拥有数与 100 的较小值。

## 动态资格和页面上下文

- 每次判断读取客户端运行时 `TableItem` 与同一 `GetTableItemUseList`：物品类别为 5、至少有一条选项，并且每个选项均为一个 `frag_*` 与正整数数量组成的 `id,quantity`。此格式与原选择页的显示解析相容。类别 2、整卡箱、混合或空奖池保持原样。
- 固定 80 包中的满足条件条目为 `item_120`、`item_121`、`item_122`、`item_156`、`item_348`，选项数依次为 3、3、12、3、3。未来资产表变动后按实际表重新计算；服务端路径和新条目是否允许批量使用需共同核对。
- `SetItem` 完成原来的初始化后，对合格物品调用原 `UpdateDetailParts` 打开现成数量组，将 `totalHaveNum` 限制到 100。原 `curSelectNum=1`、显示、按钮及增减算法均沿用；箱子不足时上限为原库存。
- 原打开选项 `id=...` 对合格箱追加本次 `fragmentCount85`。`Page.ParseOptString` 每次 `Open` 清空并重建 options，`BagChoseReward.DoOpen` 重置选项；确认时按同一页面的 options 和物品 ID 取数量。非目标箱与缺少参数的普通入口继续发送 1。
- 成功沿用 `UseItemRecv` 的服务端 `cost` 删除、`get` 入账和原奖励窗；失败分支没有本地消费。原 `HttpUtil.Send` 的忙碌检查、发送遮罩与 `HTTPManager.ResendTimeOut` 使用保存的请求字节及回调均保持输入程序集的实现。没有新增回调去重逻辑，实际遮罩拦截、断线和超时交互须在原机验收。

## 构建与固定输入

在 85 独立树运行。`--input-managed` 和 SHA 指向实际共同候选，输出目录必须是新目录；禁止覆盖冻结的 84 候选。SDK 默认从当前树向上查找 `.tools/native-batch`，也可以显式指定 `--tool-root`。

```bash
cd /Users/lirui/LR/hyjs/build/fragment-box85
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_fragment_boxes85/build.py \
  --tool-root /Users/lirui/LR/hyjs \
  --base-apk /Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk \
  --input-managed /absolute/path/to/combined/Assembly-CSharp.dll \
  --input-managed-sha256 <exact-64-digit-sha256> \
  --output-directory /absolute/path/to/a/new/overlay
```

固定参考 APK SHA-256 `25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a`，其原 Main SHA-256 `8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437`。固定原 `TableItem` 文本 SHA-256 `3b426085b94332bb40be7e0921f13efb9da822a063af68aa1ab5da68258b6088`，`TableItemUse` 文本 SHA-256 `7e25fe7a4eef44dab17a661a2b936e5b55985428cf8878114bc238d9644e317d`。原包按需读取表和依赖，不复制整包。

参考资产 SHA-256：`tableitem` bundle 为 `39e85c0c7433e7084f559df3082a151c8eaa37683f93084faeeb5bc03c96ad00`，`tableitemuse` bundle 为 `07e4d4dcbf315f7315f5b78a772e25f60a9e1a7a0c15998162769ca84a691651`；背包 prefab 资源 `3da514f2b3f0dfd46a603ecd070e226b` 为 `5ba3d528e996d00945cf65fb32ee714086fdb8abcd78128c570bf165a938cded`，选择页 `ba869c2acec1c5f458604240119a1c01` 为 `baa91e946dc94a3e8a88db18953b8be2590e9fc4f1506fbdb91a0bc3b37f12aa`，网络遮罩 `8ca9d48757eae8d43bce3ab6f724537a` 为 `21749fb16a92a39a63001eff3322a132bb2f07304260af2ae0a990493bb00ca7`。

工具检查输入 SHA 及 3 个原方法体 pin，已打本补丁或方法有其他改动时停止。输出包含：

- `Assembly-CSharp.dll` 和 `HYJS.FragmentBoxes85.dll`，合包需同时放入 `assets/bin/Data/Managed/`，并保留共同输入此前所有 helper 和资源。
- `manifest.json` 记录实际输入、输出、helper、表和源码 SHA，逐方法对照、ABI 与 547 个原引擎方法检查结果。
- 构建和桌面模型日志。临时 SDK 工程自动清理，不打 APK、不安装、不部署。

## 原始资源与验证

80 原 `pages/bagpage` prefab 的 `AmontControl` 已包含 `Minu10`、`Minu1`、`Add10`、`Add1`、`Amount` 以及按钮碰撞体，参见 `analysis/fragment-box85/client/bagpage-prefab.txt`。`UpdateDetailParts` 第四个参数控制 `amountControlRoot`；原 `OnAddOne` / `OnAddTen` 根据 `totalHaveNum` 更新数量。原 `ui/messagebox/netmask` 有启用的 `1500×1200` BoxCollider，参见 `netmask-prefab.txt`。这些证据属于静态资源和 CIL，尚未证明运行时视觉与点击结果。

可复验候选：

- 输入是 82 + 84 Wanted 程序集 `21cf2902f69ed560e2697991bc63a3a45a28ffc8bdc68313caba370184922655`。
- 输出 Main `f8e7fea253a5c72a524c62836e2a36d7e87cd6cca7c35b1dbbd444049ffcf49c`，helper `c410c12271fd2812ea86e826c3478bd23b1303b710123ca9b471c7ed8aa54225`，见 `analysis/fragment-box85/client/overlay-wanted84-r2/manifest.json`。
- 仅 `UI.BagMemberDetail.SetItem`、`OnPublicButtonClick`、`UI.BagChoseReward.OnConfirmBtn` 三个方法变化；其余 26,943 个有方法体的方法及其中 547 个 `Battle.Engine` 方法与输入逐字节一致。原字段、方法、类型定义、继承的类型/成员引用及作用域保持。helper ABI 和其对 `UI.Page`、`Dict.*` 的原主程序集类型绑定经 dnlib 和旧 Unity profile 编译检查。
- 桌面模型使用固定 80 表和实际 helper 源码，32 项断言覆盖 1、10、32、100，库存 32、上限 100、选择参数、关闭重开/换箱、非目标箱、失败不消费以及超时原字节重发的模型。模型没有运行 Unity、原客户端 HTTP 或真实服务器。
- 原方法、网络方法和补丁后的 CIL 分别见 `original.il.txt`、`network-page.il.txt`、`http.il.txt`、`httputil.il.txt`、`mask.il.txt`、`patched.il.txt`。已核原 80 与该输入的 3 个目标方法、`DoOpen`、`UseItemRecv`、`ParseOptString`、两个 `HTTPManager.Send`、`ResendTimeOut` 和 `HttpUtil.Send` 方法体相同；最终共同输入仍须检查相关生命周期与原网络约定。

首轮结果最初误写入主工作区 `/Users/lirui/LR/hyjs/analysis/fragment-box85/client/overlay-wanted84-r1/`，已迁到 85 树；同 SHA 的 r2 复验后清除重复 DLL。`initial-main-workspace-manifest.json` 保留当时路径和证明，内含的历史 payload 路径现已失效；主工作区误写的源码及空目录已移除。

## 82 + 本聊天 84 + 85 组合检查

在 85 独立树，将 82 + Wanted + 突破标签 Main `1e4486f08e1e65005462d3482c26806ecce9cb45bf3046160cd5464d0e9fa32f` 先叠扫荡，输出 `ec74f1bf5d8e5d794d8e0329cba075548d0240a9748193207c3b93e06d8cfd95`；再叠碎片选择箱，输出 `01ab4f2c66d2b5f2bac3a74c70f90e1d51c36ac24ffa8a89e74a426e436c7dbd`。扫荡和碎片模型均以对应实际输出 SHA 运行，扫荡模型也在最终 Main 上再次通过。

- `analysis/fragment-box85/client/combined-82-full84-85-proof.json` 保存输入链、逐方法 SHA、两处突破分支、全部 52 个直接及 helper 间程序集引用和来源 SHA；引用解析无缺项。
- 试炼 `TabPointShop` 2 个方法、Wanted 4 个方法、突破 `UI.BreachPage.UpdateAll` 1 个方法中 `Critical1/Critical2` 两处阈值分支、扫荡 4 个方法，均与对应前序产物逐字节一致。末次叠加仅 3 个碎片方法变化，26,943 个其余方法与输入一致，其中 547 个战斗引擎方法保持。
- 合包时除最终 Main 和 `HYJS.FragmentBoxes85.dll` 外，须使用 82 的 `SoulFlowerGGOShop.dll`（`2303449f69473f504a70fef4d988e72d35d3a856026b6133a33651f109fb7e50`）和 Wanted 的 `HYJS.Feedback65.dll`（`b71a004b326b02a52b689a599542af3e9289f31130301cb04da2f112baf68e83`）；精确外部来源见 proof 的 `payloadFiles`。80 原包的扫荡 helper `SoulFlowerStaminaSweep.dll`（`799575040c000da825d885c0f43d829d2725dcdcc6113f33eb560e19e97c3358`）及其余原包依赖继续保留。80 原包的旧 `SoulFlowerGGOShop.dll` SHA 为 `5cb8aeaa84a9097f16845e117a98e9f13cae157db5a8ba49fd2ec3eed3c68d86`，应由 82 版本覆盖。
- 突破文案的参考 `tabletext` SHA 为 `221bcf6398bf0403806236fee30d17bb8cf6f6d27d841abc0cf7aaee8de845e6`。这是参考资源；最终合包需按 `tools/client_breach84/build.py::apply_overlay` 在共同 `tabletext` 上定点合并，避免覆盖其他批次资源。本次只完成共同 Main 及其依赖证据。

目前没有包含 83 的最终共同 Main；83 固定后须以其真实 SHA 重跑相关工具，核对全部既有 helper 和资源，并在隔离环境验证实际请求、扣箱、所选碎片和冷恢复。真实服务器联调、原机布局、按钮、遮罩、失败/超时与重复点击均未在本次执行；未打包或发布。
