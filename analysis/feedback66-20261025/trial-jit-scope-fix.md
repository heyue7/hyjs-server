# Trial66 原按钮 JIT / HttpHandler 类型作用域修复

## 已证实根因

原机一次性反射调用原 `UI.RushFightReadyItem.OnFightBtnClick` 得到 `TargetInvocationException -> TypeLoadException: Could not load type NetWork.Layer.HttpHandler`。证据为本目录 `trial-native-entry-diagnostic.json`。此前无54501且runtimeLoaded=false，符合原入口在执行Click前JIT失败。

R6 Main 新增 TypeRef RID379 为 `NetWork.Layer.HttpHandler`，其 ResolutionScope 错误指向 `ModuleRef HYJS.Trial66.dll`。真实 HttpHandler 定义在 Main（TypeDef RID3890），helper自身对它的引用也正确为 AssemblyRef Assembly-CSharp。`Importer.Import(helperMethod)` 遇到 helper -> Main 的循环引用后，把 WrapStart/WrapEnd 的类型重定向到 helper module；Main因此无法解析签名。

APK内Main/Trial66/Commerce66均与R6overlay SHA相等且各仅一个zip entry，Main AssemblyRef HYJS.Trial66与helper AssemblyDef名称/版本一致，排除了打包缺失和程序集身份不匹配。

## 最小修复

仅改 `tools/client_feedback66/TrialInjector.cs` 的Hook引用构造：保留导入helper类作用域，显式用Main CorLibTypes构造primitive/object签名，HttpHandler直接绑定Main原TypeDef。不修改547引擎、原目标指令/branch/locals、TrialRuntime业务恢复源。

同时加强 `tools/client_feedback66/build.py`：对所有新增hook MemberRef解码签名并检查typed参数/返回值直接指向Main HttpHandler TypeDef；对新增的Main-native TypeRef拒绝外部scope。覆盖11 Trial hooks及Commerce hook共12种签名，不只检查原目标IL字节。

## 验证

- `build/feedback66-overlay-r7` 两helper/两injector编译、注入成功。
- 26935个非目标方法raw CIL/headers/locals/EH严格相等；547原引擎CIL/EH严格相等；原definition signatures/field RIDs、原目标instructions/branches/locals和append-only metadata检查通过。
- 新增12种hook签名结构检查通过；新增native/helper错误scope为空。
- R6负例被同作用域约束准确拒绝：RID379/HttpHandler/ModuleRef/HYJS.Trial66.dll。
- R7 WrapStart/WrapEnd签名均从旧 `00011285ed1285ed` 改为 `000112bcc812bcc8`，其中bcc8 codedTypeDefOrRef解码tag=TypeDef、RID3890，直接引用Main定义。
- 当前阶段未在手机运行修复候选。Root需重包并核原物理按钮54501、54502、自然战斗和End/恢复；静态与构建通过不等于原机通过。

R7 Main SHA256: 8d16d6bf278f547b9ec7accdc08de7dee0f8aaa4775aae3aa5aacafa8ded894a。

构建临时目录由builder自动删除；R6为原机失败证据仍保留，R7为当前待验交付。没有访问生产/DB或操作手机。
