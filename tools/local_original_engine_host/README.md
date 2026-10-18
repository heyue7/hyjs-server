# 本机原 Engine 最小宿主

已用现有.NET8在本机执行两个原Daily输入，原游戏Assembly整文件SHA保持`ee14d5df…`，547方法和DAO均未改。全部162表先与原APK TextAsset逐字核对，再由原DAO初始化。原caller/NativeBatchRunner源未修改，输出仍为原native JSONL。

产物及失败：`analysis/growth-first-20261016/p6-local-headless-poc-r1/`。`delivery.json`固定源码、程序集和证据；`comparison-r2.json`两场PASS。与手机v4的314条历史观测逐字段严格相等，无容差、无数组排序。比较器只明确隔离历史未采的新增字段/事件：damageFactor、skill事件、AI通知及原BattleOperate等；这些新增观测保留原文件，尚无该两场旧原机对应值。首份比较报告因历史缺damageFactor报字段集差异，保留为`comparison-r1.json`。

仅三个非计算Unity外围适配：Application.isPlaying返回false；AssetBundle.Unload/TextAsset.bytes一旦触达即抛NotImplementedException。它们只修改原UnityEngine.dll三个MethodDef的RVA/ImplFlags，复用原常量false/throw方法体；全部其他字节及既有方法IL/EH保持。原表reader走原editor文件分支。早期dnlib重写引起TypeSpec token变化而在验证前停止，后改原字节补丁；CoreCLR首次reader JIT还会解析未执行的两个native API，因此追加这两个fail-closed边界。没有扩大到客户端模拟。

复现必须用全新输出目录，不覆盖已固定证据；仅调用私有现有SDK，无restore/下载：

```sh
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/run.py --output-directory analysis/growth-first-20261016/p6-local-headless-poc-r1/replay-new
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/compare.py --cases analysis/growth-first-20261016/p6-local-headless-poc-r1/replay-new/runtime/cases --output analysis/growth-first-20261016/p6-local-headless-poc-r1/replay-new-comparison.json
```

本POC没有执行Go13族矩阵，没有建立新的生产权威、扩大人物/模式资格或证明每个547方法分支都执行过。当前可用范围只是这两个Daily的历史轨迹一致。新机制族、typed入口/多波与原驱动边界仍需单独锚点；UI/资源/回包仍需原机。下一单元可把此JSONL接入已有纯派生与严格比较脚本，再由root合批运行现成Go比较，不需要改Go服务架构。

失败尝试的可再生runtime已清理22,761,561字节；保留其日志、manifest和原patcher源码，成功runtime及完整轨迹留作当前交付。未用Go、SQL、手机或新SDK，无后台任务/资源持有。

## P7独立六例（后继单元）

`--case-set p7`只选已固定Trial9四例/Awake16两例，按原SHA复制wire/typed并恢复原manual driver/policy。`p7-rush`、`p7-awake`可单选；输出位于各组`cases/<group>/highest`，原RunCase(path,false,false)自行分派typed入口。Program第二参数为明确case目录。原162表、caller和三个Unity适配保持，增加的宿主设置只是原managed `Debug.logger.logEnabled=false`，不写Unity方法。未使用的daily默认模式继续保留；本批未重跑Daily。

```sh
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/run.py --case-set p7 --output-directory analysis/growth-first-20261016/p6-local-headless-p7-r1/replay-new
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/compare_p7.py --run-manifest analysis/growth-first-20261016/p6-local-headless-p7-r1/replay-new/manifest.json --output analysis/growth-first-20261016/p6-local-headless-p7-r1/replay-new-comparison.json
```

本批结果是**4例全schema4精确通过、1例失败、1例未执行**，见`p6-local-headless-p7-r1/full-schema-comparison-r1.json`。比较器不剔字段、不改顺序/容差。Trial长战002首次差异是平台`ai_notify.data.stack`尾部；后续到23轮出现真实caller推进差异并120秒超时。原caller的StackTrace判别所需AI.Update栈帧在热运行中消失，怀疑CoreCLR tiered JIT内联，尚未做独立运行时配置验证。003没有运行。Awake两例使用同份host独立执行后完整通过。不得将此批称六例或完整13族通过，不能把本机输出冒原手机capture。原日志ECall首FAIL、后继超时及独立Awake出处均保留；精确阻塞与下一接口见该批handoff.md。

## 固定宿主运行时（最后一次有界实验）

后继`p6-local-headless-p7-runtime-r1`已经只重跑原002/003：原DLL、caller DLL、DAO、162表和三Unity适配均保持，设置`COMPlus_TieredCompilation=0`、`COMPlus_JitNoInline=1`、`COMPlus_JITMinOpts=1`及runtimeconfig `System.Runtime.TieredCompilation=false`，两场8.147140秒正常结束。其2904/2919事件的全部战斗数据、时钟、AI控制、HP/AP/RNG、原导出、final protobuf、终末和结果精确一致。完整schema仍为**FAIL**：两场共16个`ai_notify.data.stack`保持手机/反射宿主身份差异，完整原文件未删除/改写。`comparison.json`分别列出`fullSchemaStatus=FAIL`和`battlePayloadStatusExceptExplicitPlatformStack=PASS`，不把二者混为完整通过。

`run.py`现在默认把这些设置仅传给host执行进程；C#编译器和外部环境保持。此前4例没有重跑，不声称它们已在这个新运行时配置重新验证。保留默认JIT下的原超时与完整schema首FAIL。纯批量开发可复用下列干净CLI（换全新目录），然后严格比较；无需APK/服务重发：

```sh
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/run.py --case-set p7 --output-directory analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1/reuse-new
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/compare_p7.py --run-manifest analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1/reuse-new/manifest.json --output analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1/reuse-new-full-schema.json
```

新运行不会把平台stack差异变成全schema PASS。`runtime_probe.py`/`compare_runtime_probe.py`用于复现这次有界两长战实验并分别报告平台身份与payload；其输出独立绑定当前原手机SHA。当前宿主仍限定已登记case set，没有任意输入或生产权威开关，不替代原机UI/资源验收。

## 实际最高觉醒003存档：一个新增family

`growth-awake003`仅含既有`p5-awake-highest-actors-20261014.json/hero_003/camp0、camp1`，对应原wire156/157与手机priority-v45的010/011压缩轨迹。真实P5冷重载存档为level100/awake2/fuse100；两wire的subject与该存档BattleHero除原场景pose2→1外逐字段相同。初始AP100、两方阵容、seed42/max10/原因素1保持原诊断前提，不称当前账号自然PVE或新人物权限。

```sh
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/replay.py --case-set growth-awake003 --output-directory analysis/growth-first-20261016/p6-local-headless-growth-awake003-r1/reuse-new
rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/compare_growth.py --manifest analysis/growth-first-20261016/p6-local-headless-growth-awake003-r1/reuse-new/manifest.json --output analysis/growth-first-20261016/p6-local-headless-growth-awake003-r1/reuse-new-comparison.json
```

`replay.py`复用已编稳定runtime，核每个DLL及原162表SHA，不编译、不拷大源树；仅两份wire进入host。原wire携带旧Go诊断optRecord40操作，保留原字节；两EnginePlayerAI自动driver不执行该记录，它不成为原网络End或当前回放黄金。

两例6.384626秒完成；每场817条历史事件在明确的新旧观察合同下顺序/数值精确一致，包括303条完整state和全部200tick/回合exports、40skill_press，以及现caller完整记录的40skill_begin/217skill_cast字段。完整schema仍FAIL：旧serializer扩展的技能对象图未在现caller重采；新schemaRevision、damageFactor、AI栈/进度、实际finalRecord等历史未采。`comparison-r1.json`逐路径列出这些不可比较观测，未改原caller或原日志、没有容差。新增finalRecord和AI栈不计为原机对照通过；本批没有Go13族或P6权限变化。
