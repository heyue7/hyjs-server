# 原 Type33 标题补丁

仅修两处原界面标题，显式启用，输出一个 `Assembly-CSharp.dll` overlay，不打包或安装 APK。

- `UI.UIActivity::InitTabs`：Type33 标签原 `String.Empty → TableTextBlo.GetText` 返回空标题。
- `UI.CopyBenefit::PrepareData`：Type33 原误用强者招募标题键；Type13 与 Type31 分支保留。

原 `TableText.tsv` 没有“充值双倍/首充双倍”键。两处使用直接 `ldstr 首充双倍`，并把紧随的五字节 `GetText` 调用换成五个 `nop`。每个方法仅允许十字节区间变化，原分支、方法头、局部签名和异常区原样保留。

```sh
rtk proxy .analysis-venv/bin/python -B tools/local_activity_title/build.py \
  --base-apk build/hyjs-local.apk \
  --base-proof analysis/p7/awake-assets-ready-r2-20261016/apk-build-r1.json \
  --output-directory analysis/growth-first-20261016/first-double/title-overlay-r2 \
  --enable-local-activity-title
```

输出目录必须不存在。当前工具严格绑定 qualified `b77673…` APK、输入 DLL `37955adc…` 和原两方法哈希，拒绝未知版本或重复应用。未来资源桥变动时须先复核并显式更新基线；不会自动放宽哈希。

验证覆盖原全部其他方法的完整原始方法体（含方法头、IL、异常区）及547个继承引擎方法，约束边界为输入的已授权客户端。原九处诊断日志适配属于继承基线，不声称与最初原 APK 全程序集相同。

最终交付：`title-overlay-r2/manifest.json`。早先单标题 `title-overlay-r1` 是保留的中间证据，不用于合包；其工具源码快照保存在其 `source-tools/` 内。
