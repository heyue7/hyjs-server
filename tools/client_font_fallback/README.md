# 原包字体补缺字

`build.py` 从固定原 APK 的两个内置 TrueType 字体重算补字资源，不读取分析目录字体或既有 manifest，不使用外部字体，也不修改 DLL、UILabel 绑定或原 APK。

## 使用

依赖本项目 `.analysis-venv` 中的 UnityPy 与 **fontTools 4.67.0**。工具运行时核对 fontTools 版本；本轮验证使用 UnityPy 1.25.4。

```sh
rtk proxy .analysis-venv/bin/python tools/client_font_fallback/build.py \
  --base-apk build/hyjs-local.apk \
  --output-directory analysis/playability31-20261020/client/fonts-fallback-r1
```

输出目录必须不存在。工具拒绝覆盖，输出资源 entry 原路径及 `manifest.json`。该命令只生成资源覆盖，不签名、打包或安装 APK。

打包器可以导入 `build_overlay(base_apk)`，取得 `(entryToBytes, proof)`；每次调用均从输入 APK 重新校验和计算，不能用旧 manifest 作为补字输入。函数会短暂包装 UnityPy `ObjectReader.from_reader` 捕获真实对象表位置，并在 `finally` 恢复；调用应串行，期间不要并发执行其他 UnityPy 解析。

## 固定输入与覆盖

- APK SHA256：`9fec499af7473f5fd05da31670e27b3e9eb270ba9b0c9d52bd6f3238e1425285`。
- 原 sharedassets0 重组 SHA256：`000d67a882772004f08bb9defc16e0cd9e42e290c4f3251a8ca4d90334dec6b8`。
- Font190 Numbers SHA256：`d6c6319ead092fa606fe8942544d666c547d1469b6e23aa7abc26666a563859c`。
- Font191 Words SHA256：`c14a2c7606b46548f9a6af86a5c641541831a084d244f4c1533709ce87849371`。

Numbers 保持 UPM875，从原 Words 补全部7455个缺失码点，Unicode覆盖967→8422。Words 保持 UPM256，仅从原 Numbers 补 `©` U+00A9、`•` U+2022，Unicode覆盖7741→7743。

每个目标字体保留所有旧 glyph ID、原 `glyf` 切片字节（含指令）、所有旧 `hmtx`、所有旧 cmap 映射、名称和原 hint 表、UPM 与行高/基线参数。只缩放 donor；新增 glyph 独立重命名，递归复制复合字形依赖、重写引用并清空 donor hint 指令。重载后逐项验证旧字形与新 donor 几何、度量。

扩容必需的派生变化在 proof 中逐字段列明，包括字形数、水平度量容量、字形/cmap/loca/post内容及 checksum。Numbers 的新增轮廓要求 head.xMin -13→-21、head.yMin -150→-215，hhea.minRightSideBearing -20→-116；这些是整体边界统计，原数字轮廓、advance/lsb、ascent/descent/lineGap/caret保持。Words 仅新增两字形的容量和 checksum 变化。

## Unity 资源范围

工具只读取 sharedassets0 的11片，合计约11MB，不解压整包。两个 Font 新 payload 追加到原 serialized 文件尾；根据真实 version15 对象表解析位置，只改文件头 fileSize 与190/191记录的 offset/size。全部513个非目标对象的 raw 与 metadata严格保持；两个Font除 `m_FontData` 外全部字段保持，包括字号、行距、ascent、material、texture、fallback与PPtr。

返回这些资源 entry：

- `assets/bin/Data/sharedassets0.assets.split0`：文件头与两条 Font 对象记录。
- `assets/bin/Data/sharedassets0.assets.split10`：保留原末片前缀，追加新 payload 到1MiB。
- `assets/bin/Data/sharedassets0.assets.split11`～`split15`：剩余新 payload，末片以外均为1MiB。

所有分片沿用Unity读取器的1MiB边界；split1–9逐字节不变。新片继承原片的APK存储方式。首个候选将末片扩展到6MB，原机停在启动画面；修正分片后原机成功登录并进入雪原，证据见批31记录。工具不触碰另外三处已验证 UILabel PPtr 修复；打包器应继续合并其独立覆盖。

`proof` 记录输入/输出/源码 SHA、依赖版本、每个原glyph和新增donor的校验、全部515对象比较、变化字节位置和全部split SHA。任一输入、版本、字形、Unity字段或字节范围不符合条件即失败。

## 验证边界

已做固定原包重算、逐glyph/对象重载、重复输出字节一致、错误输入SHA/依赖版本拒绝与已有输出目录拒绝验证。固定分片整包已通过原机启动、登录和雪原文字代表检查；各页面的完整游玩验收见批31记录。新增覆盖限于这两个原字体的既有码点，原资源仍缺少TableText中的9个生僻字，不承诺全部Unicode字符。
