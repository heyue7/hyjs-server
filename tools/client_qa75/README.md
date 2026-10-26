# 独立本机QA75客户端工具

准备阶段只测试工具。等主代理冻结最终APK75（包含已确认UI与HTTP时间戳修复）后，由主代理显式构建和安装。本工具不连接设备、服务或数据库，不修改原客户端工具、正式APK、当前/回滚指针，不导入生产账号资产。有限测试前置只属于本机隔离测试账号。

输入必须给出 `--base-apk`、`--base-manifest`、`--base-manifest-sha256`。冻结证明须包含真实 `binarySHA256`；证明本身的SHA与APK内容两次核对，拒绝变更或符号链接。该证明只绑定主代理选定输入，工具不会自行宣称该APK已经完成手机验收。UI、DEX时间戳改动、原战斗代码及资源全部继承此输入。

overlay只改四个已核对原方法中的一个ldstr操作数：ServerConfig.GetServerUrl、Battle.FieldPVEWorldBossBattleOver.End、Room.RoomNetField.SCCurrentPosition、VoicePlugin.InitGVoice。来源地址严格从冻结APK的assets/local-wallet.json读取，目标只接受 `http://127.0.0.1:18861`，钱包JSON同步这个origin。沿用已参数化QAInjector，不绕过旧remote工具的固定SHA检查，不修改旧工具。每次用新输入做完整逐方法审核：其余方法raw body完全相同、547个Battle.Engine方法完全相同、方法布局/分支/异常区不改、无新增类型/方法/字段。

TCP不在客户端硬编码：隔离服原SCUserLogin应返回127.0.0.1:18862，ADB reverse由主代理处理。工具不执行ADB，也不启动服务。

打包复用现有build_multiclient二进制AXML解析与字符串池逻辑，只把已核 `.local` 包名改成独立的 `com.yunchang.djsy.ewan.multi`，保留75版全部当前标签与其它XML。原工具期待旧标签/两个label，75版已有新标签和三个label，因此新wrapper用同一解析逻辑单独限制为package一处变化，不替换旧工具常量。仅替换Assembly、wallet JSON及二进制Manifest三项；全部其它ZIP载荷，包括所有DEX、资源，按原压缩字节逐项核对。证书必须和输入一致，执行zipalign/apksigner/aapt2校验。签名材料只通过显式 `--keystore` 读取，证明不记录秘密。

工作树缺少私有运行时和签名文件；可直接复用主工作区的既有运行时，不复制材料、不建立全局符号链接：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_qa75/build.py \
  --base-apk <主代理冻结APK75绝对路径> \
  --base-manifest <冻结证明绝对路径> \
  --base-manifest-sha256 <冻结证明SHA256> \
  --origin http://127.0.0.1:18861 \
  --runtime-root /Users/lirui/LR/hyjs
```

默认新目录为analysis/growth75/qa-local-overlay，须不存在；可通过 `--output-directory` 指定growth75下另一个全新目录。目录内生成overlay、manifest和日志，不生成APK。编译使用本机.NET8/dnlib，清空NuGet源并禁用审计/遥测/共享编译，不下载依赖或访问源。

随后由主代理打包：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_qa75/package.py \
  --base-apk <同一冻结APK75绝对路径> \
  --base-manifest <同一冻结证明绝对路径> \
  --base-manifest-sha256 <同一冻结证明SHA256> \
  --overlay analysis/growth75/qa-local-overlay \
  --keystore /Users/lirui/LR/hyjs/.local-signing/debug.keystore
```

输出为该overlay目录的hyjs-qa75-local-multi.apk与apk-proof.json；已存在时拒绝覆盖。只用自己的临时目录与打包锁；成功和失败清理临时编译与APK中间文件，失败保留小日志。证明记录源文件SHA、输入与输出SHA、547引擎及压缩载荷校验，明确未安装、不可提升为正式包。

离线工具保护测试（使用小型合成ZIP/AXML，不碰真实APK）：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python -B -m unittest tools.client_qa75.test_tools -v
```
