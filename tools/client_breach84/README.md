# 突破说明定点覆盖

该工具只从 `break_through_desc` 删除“突破值每天24:00清零，”，保留 `[000000]`、`当前有{0}直接升级` 与 `[-]`。输入 APK 及 SHA-256 必须由合包方明确提供；生成物只用于 overlay，不构建或签署 APK。

```
/Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_breach84/build.py \
  --base-apk /absolute/path/to/input.apk --base-sha256 <64-hex-sha256> \
  --output-directory build/breach84-overlay
```

合包工具调用 `apply_overlay(base, out, payload, pins)`，该函数先重算并校验参考产物和源文件，再读取 `payload` 已有的 `tabletext` 并逐行定点修改；其他批次已修改的行和其他资源均保留。若目标行已被改变，直接报错，由合包方排查冲突。不要将参考 `tabletext` 文件直接覆盖其他批次的同名资源。

`manifest.json` 记录 APK、输入/输出资源与文本脚本摘要，逐行和其他 Unity 资源对象检查结果；它仅代表静态资源验证，原机结果另行验收。

## 概率标签临界点

`build_main.py` 对已合并的 Main 做独立覆盖。原 `UI.BreachPage.UpdateAll` 的 `Critical1`、`Critical2` 两处 `bgt.un` 改为 `bge.un`；包含 `Critical3` 的经验段继续显示原“较高概率”标签。两个分支的位置和目标均从实际输入方法验证，输入目标 CIL 与原80版不同会拒绝，以免覆盖新实现。表格、文案、字体及布局保持原样。

```
/Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_breach84/build_main.py \
  --input-managed /absolute/path/to/composed/Assembly-CSharp.dll \
  --input-managed-sha256 <64-hex-sha256> \
  --output-directory build/breach84-label-overlay
```

合包方也可在已有 `payload` 包含实际叠加后的 Main 字节时调用 `apply_overlay(payload, input_sha256)`；该函数检查完整输入 SHA 与目标方法，再直接对已合并内容写两个 opcode。每个输入都要重新生成，不能把参考 Main 覆盖到其他批次成果上。

`test_main.py --case <input-dll> <overlay-directory>` 可重复传入多个输入；它重新核验完整 DLL 差异、其余方法和547个引擎方法，并用本机 .NET CLR 执行由两处实际 opcode 构造的有限比较方法，逐阶覆盖所有合法整数经验。该检查不加载Unity游戏，不计原机界面通过。
