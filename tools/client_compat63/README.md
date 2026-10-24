# 批63 Android启动兼容增量（独立产物）

固定输入正式62 APK SHA256 `fe427f13e8683b434bf7d2756b2dbd4a6f42746d0d6c66c86264182252704580`。本目录不打包、不签名、不安装、不操作原机。

## 已实现

新增 `com.hyjs.compat.StartupActivity extends android.app.Activity`，不继承或以class literal引用Unity/ClientUpdateActivity。后台线程调用原公开 `LocalAssetOverlay.synchronize(applicationContext)`；成功且Activity恢复前台时，显式Intent启动原 `com.hyjs.wallet.ClientUpdateActivity`，随后finish。原Wallet再同步一次，全部校验保留。正常路径无需确认。

失败只显示固定文案与“重试 / 退出”，不显示原始异常、路径、栈或凭据。无资源删除逻辑。取消/销毁阻止后续handoff；一轮同步与一次成功handoff有独立guard；重复launcher从已有task启动时finish；已handoff状态恢复时finish。Activity具体Android生命周期和画面待原机验证，host probe不声称覆盖这些行为。

AXML补丁克隆原Activity的主题/label/横屏/configChanges等属性给新增Activity。原Activity属性、Unity metadata、exported保持；仅将唯一MAIN/LAUNCHER intent-filter搬至新增Activity，新增麦克风uses-feature required=false。不更改权限/minSDK/targetSDK/版本/资源表。

## 确定的原资源缺陷与修复边界

正式62 `smallassetmanifest` 实际564315字节/SHA `fac59a57d1a146ad36cd87dcbc05a0f43fc516f6d81393c858d0cc94b087130f`，原JSON与DEX pin仍563672/cfddb4…；fresh cache会失败，匹配旧pin的缓存可跳过APK读入。这是既有缺陷，不可绕过校验。

已获主代理批准：原Java源码只同步这一Pin，JSON同一行同步；DEX仅此类clinit的同一Pin语义允许变化，其他所有方法指令/异常处理、类API/flags/索引保留。由于DEX string_ids排序，禁止将cfddb4…原位改为fac59…；需要经审查的有界clinit/data结构重定位。当前唯一可接包产物为 `payload-artfix-r10/`。旧r8/r9虽D8/dexdump通过，原机ART明确VerifyError：clinit PC0x5c普通move复制String引用；旧具体解释器也遗漏类型检查。此次纠正为move-object，实际ART64 PinProbe成功初始化六Pin并退出0（证据analysis/feedback63-20261024/phone-art64-pinprobe-r10.txt）。失败旧产物只留小manifest/边界证据，不能使用。现有string/type/method索引与字符串内容全部不变，原code段尾新增304字节clinit，用已有String(byte[],UTF-8)引用构造新SHA；旧clinit保留为未引用code_item。后续data绝对offset按同一delta平移，所有ULEB实际长度逐项校验，若变宽则安全拒绝输出。map所有section完整parse、跨度与offset目标类型核验通过，473个其他方法的instructions/EH/debug内容逐字节相同。dexdump及Android D8解析验证通过。

从**实际patched DEX**定位当前clinit，运行限定opcode的具体Dalvik解释提取6个Pin，与JSON及真实APK资产的size/SHA逐项一致。此具体解释器现在跟踪引用/primitive/wide-low+high与构造器参数类型；三项负例必须拒绝：普通move复制引用、move-wide复制引用、move-object复制wide。真实ART64 probe另行核验classes2 SHA b592412b…及六Pin，仅证明Java初始化，未加载Unity或同步资源。

## 构建

```sh
rtk proxy .analysis-venv/bin/python tools/client_compat63/build.py \
  --base-apk /Users/lirui/.codex/worktrees/ggo-items-shop/hyjs/build/hyjs-local.apk \
  --output-directory tools/client_compat63/payload-artfix-r10
```

`--input-manifest /absolute/final-version-AndroidManifest.xml` 可对最终已经升级版本的二进制Manifest执行同一增量，保留版本，拒绝重复bootstrap/重复麦克风声明/非预期intent-filter。主代理也可直接调用 `tools.client_compat63.axml.patch(final_manifest_bytes)`，应在公共版本patch之后执行，不能先加bootstrap再调用旧版本patch。

## package_native接入合同

最终package步骤：验证base SHA及manifest/source/payload pins；先做原版本/QA/managed等既有overlay，再对最终binary Manifest调用上述patch；覆盖仅 `AndroidManifest.xml`、精确修复后的`classes2.dex`和`assets/local-resource-overrides.json`；追加 `classes5.dex`，若最终候选已有classes5则拒绝冲突，不自动覆盖。新增DEX只含本目录11个com.hyjs.compat类（无Unity类型引用），与全部原DEX无重复类；保classes.dex/classes3.dex/classes4.dex字节相同。原managed/native均与兼容增量输入相同（主代理其他overlay的变化另算）。

打包后重新核6项配置/pin/实际资产一致、全DEX唯一类及non-final superclass、aapt launcher/mic/横屏/原Activity属性/版本、签名/align和最终条目白名单。原Activity元数据不可删；原launcher filter不可留第二份。当前仅payload，未集成/发布。

## 验证与限制

host编译真实LocalAssetOverlay源码与生产StartupGate，隔离Context/File/AssetManager夹具，真实APK六项资源。覆盖原错误复现、修后fresh正常、hash/IO/security失败不启动、重试只一次、无关文件保护、inflight/cancel/handoff失败guard。host org.json复用本机已有jar，无安装依赖；临时数据目录随TemporaryDirectory清理。

未验证：真实Android恢复/后台/重复launcher/错误页视觉和触控、小屏/平板/无麦克风；错误发生在原ClientUpdate/Unity后续生命周期或native SIGSEGV时，plain Activity无法捕获。旧Unity引擎仅ARMv7/x86，64位only及16KiB仍无支持证据。未做GLES2-first配置。

## 主代理构建与复验函数接口

- `tools.client_compat63.axml.patch(raw_binary_manifest) -> (new_bytes, proof)`：必须在公共版本更改后执行；原Activity保存，launcher filter移交。
- `tools.client_compat63.dex_patch.patch(formal62_classes2_bytes, actual_manifest_sha256, actual_manifest_size) -> (new_dex_bytes, proof)`：只接受已核Pin/结构合同；输出与payload的classes2.dex应全字节相等。
- `tools.client_compat63.dex_patch.layout(dex_bytes)`：全map实际section/span/offset类型检查。
- `tools.client_compat63.dex_patch.extract_pins(patched_dex_bytes)`：从实际当前clinit提取Pin数组，不从JSON推断结果。
- `build.py` 固定formal62，返回manifest `replacements`（三个覆盖项）/`additions`（一个新DEX）、`sourcePins`、`originalDexSHA256`及`dexProof`。主代理需用当前输入classes2.dex SHA与record原值比对，拒绝已叠加其他Java改动的DEX。

## r10原机故障纠正记录

此前r8的静态PASS不构成ART可加载结论，已被原机VerifyError推翻。修复后classes2除checksum外仅opcode 0x01→0x07一字节变化；304字节重定位/索引/473其他方法合同不变。生产classes5保留HYJSStartup安全Log.e（prepare/handoff、类型/因果/stack class.method line，省略异常message）；FailureDiagnostic纯Java负例验证敏感message/文件路径不输出。QA PinProbe仅对VerifyError打印消息，属于独立QA文件，不进发布classes5。

完整fresh Activity资源准备/重试/Unity正常进入仍由主代理原机验收。此处ART64 standalone通过不宣称旧Unity native在64位运行。
