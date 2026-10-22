# 客户端更新桥

当前基线为 `build/hyjs-local.apk`。新进程生成独立 startup UUID，两处登录检查与登录界面版本标签的 appVersion 编码为 `hyjs/<buildCode>/<startupUUID>`，原 resVersion 保持不变。Java 端 `buildCode`/`versionName` 在运行时通过 `PackageManager` 读取已安装包的 versionCode/versionName，清单是唯一版本来源，不会与源码常量产生偏差。`--build-code` 和 `--version-name` 为必填构建输入，buildCode 必须大于基线清单值，并与打包写回清单的值一致。

## 构建与合成

通过项目 Python 环境运行，须传入已核验的基线 SHA256：

```sh
rtk proxy .analysis-venv/bin/python -B tools/client_update/build.py \
  --base-apk build/hyjs-local.apk --base-sha256 <SHA256> \
  --build-code <大于基线清单的 buildCode> --version-name <清单 versionName> \
  --output-directory analysis/client-update-20261021/client-overlay-new
```

输出 `Assembly-CSharp.dll`、`HYJSClientUpdate.dll`、独立 `classes.dex`、`manifest.json` 和小型编译日志。编译临时文件自动清除，不产出 APK、不安装、不发布。`--input-managed` 支持在主代理已经合成的程序集上叠加；证明记录真实输入 hash。整个构建逐方法核 ABI/完整方法体，只有两处登录检查和 `UI.LoginNormalUI::Start`/`::Show` 的版本标签共四个方法允许变化，547 个原引擎方法须完全相同。再次以已接入 APK 构建时，Injector 验证已有同名桥和相同参数/返回值后保留调用；实际变化可为零，证明记录真实差异，新 helper 与 Java dex 仍由本轮输出替换。

主代理的 `package.py` 负责以下受控合成：

- 新托管程序集替换 `assets/bin/Data/Managed/Assembly-CSharp.dll`，新增同目录 `HYJSClientUpdate.dll`。
- 独立 dex 按基线已有 dex 数量选下一个 `classesN.dex`。classpath-only `WalletActivity` 编译 stub 不得进入产物。
- 原 `WalletActivity` 为 final；必须只去除定义该类的旧 dex class_def 的 ACC_FINAL 标记，重算 SHA1/Adler32，核该 dex 除四字节标记和校验头外全部精确相同，其余旧 dex 完全精确。
- 唯一启动 Activity 改为 `com.hyjs.wallet.ClientUpdateActivity`，同步清单 versionCode/versionName。新 Activity 延续原钱包 Activity 的创建、恢复、暂停行为。
- 保留原 `assets/local-wallet.json`，运行时只读取其中 origin。

## 行为与边界

`ClientUpdate.boot(Activity)`、`resume()`、`pause()`、`version()` 为固定 Java 接口。status/ack 使用反射读取 WalletActivity 真实内存 credentials；请求与结果绑定同一对象，旧会话响应丢弃。回执捕获实际展示策略对应的会话，强制退登录清理 credentials 后也不冒用新会话。凭据不写盘、不打印。

首次启动且本机未接受声明时，`boot` 先显示不可取消的原生“使用声明”，位于更新提示之上；返回键、点外部、暂停恢复和 Activity 重建都不会关闭或绕过。声明 pending 期间不轮询 status，也不接收/显示更新弹窗。左侧红色“退出”结束当前任务并退出游戏进程，不写任何标记；右侧蓝色“了解”写入本机 accept flag 后恢复原有轮询与更新流程。该标记属于设备本地偏好，跨启动和 APK 升级保留，不按账号区分。

C# `Bridge.Version(GameConfig)` 适配原 instance getter 消耗的参数并调用 `Version()`；Injector 仅将两处登录检查和 `UI.LoginNormalUI.Start/Show` 各一处原 `get_AppVersion` 的 call/callvirt 替换成相同栈形状的静态桥，原 GameConfig.Instance、get_ResVersion、资源版本校验及其余 getter 调用保留。`Version()` 创建 DontDestroyOnLoad Gate，所有场景、战斗检查及 BackToLogin 都在 Unity Update 主线程。Android UI 由 Java Handler 调度，无跨线程 Unity API。

普通提醒只在原登录界面，或已证明 RoomTown 且原 TownPage 可见、battleEngine 为 null 时显示；战斗/结算/未知场景保留待处理。batch 展示后本地持久化去重，新 batch 可再次提醒。shown 仅在 Dialog.show 成功后发送。强制提示调用原 BackToLogin 关闭网络和原游戏状态，清理钱包真实 session，并持续留在登录页。按钮“重新检查”发起真实 status 检查，服务器解除阻断才关闭提示；不退出应用。

界面沿原 SAO 灰白、青蓝、深灰文字，固定文案“请前往QQ群下载最新版”。没有群号、链接、复制、下载或跳转动作。远端标题/正文为普通不可点击文本。普通提醒按钮“知道了”，强制提示按钮“重新检查”。首次声明沿用同一浅蓝底板、字体和可滚动正文，左侧红色“退出”、右侧蓝色“了解”；正文高度按屏幕可用高度收缩，横屏和窄屏下两个按钮保持可见可点。

登录页判定遍历原 `LoginUI` 组件及其 activeInHierarchy，不依赖绝对对象路径。普通提醒在本端 buildCode>=latestBuild 时直接忽略；维护阻断继续生效。弹窗展示远端标题/正文、当前/最新版本，以及提醒、下次登录更新、维护计划/预计结束的固定后果说明，时间明确为北京时间。

当前证据仅为针对当前基线的小型 Java/D8、C# 编译、四方法 IL 修改与 547 原引擎守卫。尚需主代理完成后端真实 status/ack 联调、首次声明/普通/重提醒、登录门槛、维护、战斗延后、恢复、窄屏及原机同状态界面验收。编译通过不代表原机可用。
