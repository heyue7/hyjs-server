# 游戏前台显式注册

正式路径：原登录页 `UserInput.OnRegistButtonClick` → 独立 Java `GameRegistration.show` → 三字段现代 `EditText` 弹窗 → `POST /game-account/register`。成功只提示返回原登录，用户自行登录；不调用普通登录作为注册，不保存密码或自动创角。原 APK 的“快速注册”按钮原回调为空，原 `RegistUI` 类虽存在，登录 prefab 没有该组件实例。

`build.py` 基于固定当前 APK，只覆盖一个 managed 方法，所有其他继承方法与 547 个战斗引擎方法逐原始方法体核对。旧 `classes2.dex` 作为 D8 输入，增加注册类族；钱包源不重编。现代游戏输入法所在原 `classes.dex`、品牌、钱包配置与其余资源由组包器逐 SHA 及压缩数据保持。

`package.py` 只输出新 `build/hyjs-registration-candidate.apk`，使用现有签名身份、zipalign 和共享客户端构建锁，保护 current/rollback，由主代理在原机验收后提升。

最终构建使用 `analysis/game-registration-20261018/native-java-r2`，APK 记录 `analysis/game-registration-20261018/apk-build-r1.json`。原机记录位于同批次 `phone-r1`。

原调查 `original-ui-contract.json` 与 `original-prefab.json` 保留真实缺口。`native-r1/r2` 是已放弃的 managed 类型合并方案失败日志；`LocalGameRegistration.cs` 为该方案源码，**当前构建不读取、不合并、不执行它**。`native-java-r1` 为加入滚动容器前的历史构建，不用于安装。当前 Java 弹窗在键盘弹出时允许滚动，并保留确认校验、失败留页、重复错误、返回取消及连接/读取超时。
