# 原游戏统一输入法适配

原 `UnityPlayer.showSoftInput` 统一进入 `com.unity3d.player.s`。此模块替换原四个键盘类，保留 Unity 原生输入回调，将当前系统 IME 接入游戏内输入框，关闭全屏提取编辑页。钱包金额框由原生 EditText 使用相同的非全屏输入选项。

- `src/com/unity3d/player/s.java`：输入连接、中文组合、候选提交、Done/Back、密码和八类原输入类型。
- `patch.py`：只移除四个原键盘定义及对应 class_data；原其余 code_items、异常处理和方法编号保持。
- `build.py` / `verify.py`：Java/D8、84项输入状态检查、30461个非键盘方法逐字比较及 Android dexdump 验证。首次适配的固定输出为 `build/modern-ime-r3-20261016`。

完整 APK 由 `tools/build_wallet_client.py` 精确升级。已安装 IME 的 APK 后续只改钱包 UI 时，沿用其 `--base-wallet-proof`，不要再次传 `--modern-ime`；原 classes.dex/classes3.dex 必须逐字继承。Unity 编译 stub 不进入最终 DEX。

手机证据见 `analysis/growth-first-20261016/modern-ime/native-20261016/summary.json`：中文候选、Done、重开、Back、数字小数草稿及冷登录已通过，`mFullscreenMode=false`。Emoji 可由系统键盘提交，原游戏字体目前会显示为空白；未替换字体，未计入显示通过。
