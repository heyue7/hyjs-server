# 悬赏跳过结算窄修87

原 `BattleManager.Skip` 直接同步计算整场；原 UI 消息队列稍后才排入引擎。原引擎已结束时再收到 `51006`，`ComputeCore.FinishRound` 仍会更改最终回合。公开残血派生的原引擎对照显示，跳过击杀后回合1可被改成2，HP和胜者不变，最终操作字节改变；严格结算正确拒绝这种改写。

本工具只输出主程序集和 `HYJS.WantedSkip87.dll`，不改547个原引擎方法、伤害、奖励、次数或服务端校验，不添加UI/提示文案。

## 行为

- 在原 `BattleManager.Skip` 设置 `isSkip=true` 后、原 `EngineDefault.Skip` 前，按原FIFO和已登记delegate排空 `ViewToEngine`，保留有效操作。每轮重读队列及delegate，保留原重置、空handler和异常顺序；不消费 `EngineToView`。
- 原 `EngineMsgReceive.PushMessageToEngine` 入口只在原战斗类型8、非回放、已经跳过且真实 `ComputeCore.end=true` 时拦下迟到的51003/51006/51008/51010/51012/51021控制消息。尚未结束、非悬赏及原回放沿用原指令。
- 55011网络结算、`BattleNetOver.ParseOptRecord`、`WantedBattleOver`和原HTTP重试保持。结束记录不被修改来凑服务端结果。

## 构建与集成

在当前工作树运行，输入必须是实际共同Main及完整SHA。输出目录须全新。依赖现有原80 APK和私有SDK，不下载依赖。

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_wanted_skip87/build.py \
  --input-managed /absolute/path/to/Assembly-CSharp.dll \
  --input-managed-sha256 <complete-sha256> \
  --base-apk /Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk \
  --output-directory /absolute/path/to/fresh-overlay \
  --tool-root /Users/lirui/LR/hyjs
```

**最终APK须同时打入两个payload**：`assets/bin/Data/Managed/Assembly-CSharp.dll` 与 `assets/bin/Data/Managed/HYJS.WantedSkip87.dll`。其他helper与资源均继承实际共同输入；不能拿01ab或83示例整包覆盖86。原通信方法会解析新helper调用，缺少程序集可影响其他玩法，因此交付必须核APK内条目及散列，再验原机普通战斗的首个控制消息。

构建器检查两原目标body pin、所有非目标原方法体、547个引擎方法、原ABI/引用/元数据堆和签名行；实际注入顺序须保留 `isSkip=true → BeforeSkip → 原Skip`。旧Unity profile编译与独立边界模型同次运行。`ModelTests`以受控类验证FIFO、终末守卫、回放、其他玩法、delegate异常与队列重置，未运行Unity；它不替代真实原通信队列和55011联调。

## 证据与验证边界

- 原CIL及独立审查：主工作区 `analysis/wanted-skip87/cil/`。
- 原引擎10例、公开残血派生与原PB：`analysis/wanted-skip87/native/`。仅在初始化前把原敌方4体当前HP各设为1，其余参数保持；不代表自然玩家或手机采集。尾部原调用由宿主模拟，不能单独证明手机实际触发时序。
- 服务端新增纯计算回归 `TestP12Wanted87OriginalSkipKillAndLateFinishRejected`，同时运行旧 `TestP12Wanted86NativeRoundControlsStrictRecord`，验证正常原记录接受、原尾部篡改记录拒绝。
- 最终每份输出的实际输入、源码、主程序集/helper SHA及模型日志在其manifest目录。此前r1未排除回放，属于历史候选；最终使用含回放检查的r2或后继。
- 原机仍需检验：点击跳过前有待处理操作、残血击杀/正常打完、普通与致命一击、结算成功及冷重登、再次开战、悬赏回放与非悬赏控制。不能把离线模型或原宿主叫成原手机通过。
