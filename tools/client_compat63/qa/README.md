# 只读 ART PinProbe（主代理手机租约执行）

`hyjs63-pinprobe.dex` 只含 `com.hyjs.compat.qa.PinProbe`，不替换发布DEX、不登录、不读凭据、不调用同步、不加载Unity。Class.forName会真实执行已安装APK的LocalAssetOverlay.clinit；随后反射6个严格白名单Pin并打印path/bytes/SHA，或异常/因果类型与stack class.method/line。省略所有异常message。

主代理先从 `adb -s SERIAL shell pm path com.yunchang.djsy.ewan.local` 获得本次已安装APK绝对路径，记为下方 `INSTALLED_BASE_APK`。不要用本机APK路径，也不要用旧安装的/data/app随机目录。以下是命令模板，由主代理替换 SERIAL 与 INSTALLED_BASE_APK；本代理未执行任何手机命令。

```sh
adb -s SERIAL push tools/client_compat63/qa/hyjs63-pinprobe.dex /data/local/tmp/hyjs63-pinprobe.dex
adb -s SERIAL shell run-as com.yunchang.djsy.ewan.local cp /data/local/tmp/hyjs63-pinprobe.dex files/hyjs63-pinprobe.dex
adb -s SERIAL shell run-as com.yunchang.djsy.ewan.local chmod 444 files/hyjs63-pinprobe.dex
adb -s SERIAL shell run-as com.yunchang.djsy.ewan.local sh -c 'CLASSPATH=INSTALLED_BASE_APK:/data/user/0/com.yunchang.djsy.ewan.local/files/hyjs63-pinprobe.dex /system/bin/app_process32 /system/bin com.hyjs.compat.qa.PinProbe'
```

若 `app_process32` 不存在或系统禁止run-as执行，记录环境错误，不尝试提权/改手机全局设置。APK路径和probe文件需app UID可读；probe设为只读以兼容动态DEX加载约束。这个独立进程只验证ART类型初始化，不提供Android Application Context，无法证明后续资源同步成功。

预期正常最后一项为 smallassetmanifest bytes=564315 sha256=fac59a57d1a146ad36cd87dcbc05a0f43fc516f6d81393c858d0cc94b087130f；若有VerifyError/ExceptionInInitializerError等则先依据类型和stack定位，不接Unity。完成后仅清理这两个自有probe副本，保留手机prefs/缓存保护备份。

若ART PinProbe通过，下一步用 `payload-diagnostic-r9/classes5.dex` 替换r8仅此条目增量重签，或走package_native --compat-overlay此新目录（完整sourcePins已更新）。固定日志标签 `HYJSStartup`，可由主代理采集 `adb -s SERIAL logcat -d -s HYJSStartup:E`；不会将原始异常消息显示到错误页。正常业务日志/凭据不在本probe输出范围。
