# 原生登录与注册面板

登录和注册共用 `LoginStyle` 的图片背景、颜色、输入框和按钮。图片放 `assets/panel.png`，最终 APK 条目为 `assets/login-ui/panel.png`；可在编译后由集成人补入同一条目。所有标题、输入文字和勾选项由原生控件按设备分辨率渲染，不把文字烧进图片，也不缩放勾选项。

- `LoginCache.show` 显示原生 Dialog；仅显示记住密码；自动登录选项隐藏，旧缓存的自动登录开关忽略，启动/ready均等待手动登录。输入由 Android UI 线程 watcher 写入同步静态字符串，Unity线程只读字符串。
- 新单方法 managed prefix 只在 `LoginCache.hasForm` 为真时，将表单值写入原 `UIInput`。随后完整执行原 `UserInput.OnLoginButton` 验证、登录协议、attempt guard、成功缓存和失败路径。
- 表单隐藏或关闭即清空可见状态和临时字段。返回调用原 `OnCloseButtonClick`，注册调用原 `OnRegistButtonClick`，未另建登录接口。
- 注册沿用原 `register` HTTP 实现与账号/密码限制。失败留在注册面板；取消返回登录；成功临时预填新账号，仍需原登录成功才写加密缓存。进入注册取消排队的自动登录尝试。
- Dialog 高度最多360dp，内容用 ScrollView；IME 使用原生 EditText、NO_EXTRACT_UI 和 ADJUST_RESIZE。注册后台网络线程只接收在UI线程捕获的不可变字符串。

```sh
rtk proxy .analysis-venv/bin/python tools/login_ui/build.py \
  --base-apk apks/build/hyjs-local.apk --base-sha256 <固定APK哈希> \
  --input-managed <已合并其他覆盖层的DLL> --input-managed-sha256 <固定DLL哈希> \
  --output-directory analysis/login-ui-new-candidate
```

该工具输出 `Assembly-CSharp.dll`、`classes2.dex`、公开ABI记录及 manifest。它仅重编 wallet/account classes2 的全部现有Java源和新增样式；Unity stubs只进编译classpath，不进入DEX。原 `classes.dex` 与 `classes3.dex` 必须按manifest保持原样。

验证比较原真实公开类的 public/protected 字段与方法ABI；D8生成的 synthetic lambda类按内部实现处理。原主DLL仅 `UserInput.OnLoginButton` 增加29条前缀，所有原验证/网络指令后缀、其他managed方法及547个Engine方法必须保持。最终仍需手机验收显示、IME、取消、注册返回、手动登录及冷启不会自动登录。
