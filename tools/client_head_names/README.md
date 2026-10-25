# 头顶姓名字体候选覆盖

仅支持正式69 APK SHA256 `d60dd8b6a9de9b11d8a673d240f19f3cc16d33956cdede386794cc26e2f71f8d`。此工具生成独立资源候选，不打包、签名或安装APK，不修改程序集、Java或输入APK。

```sh
rtk proxy .analysis-venv/bin/python tools/client_head_names/build.py \
  --base-apk build/hyjs-local.apk \
  --output-directory build/head-names70-overlay
```

输出目录须不存在。打包器可导入 `build_overlay(base_apk)`，取得 `(entryToBytes, proof)`，只替换返回的单个entry；其余ZIP entry沿用原数据。输出manifest不能代替输入校验。最终完整APK仍须独立核对全部非目标entry、程序集和dex的SHA。

目标entry为 `assets/bin/Data/c09f4969afcb1ed43858888756b9f3fe`，原SHA256 `18a791169a577c0e48bbcd6d24c3ccd40c1967743821b6639986479afe0c09eb`。RCUIPlayer3D内6个TextMesh（29–34）由空字体改用sharedassets0 Words Font191；对应6个MeshRenderer（20–22、24–26）由内置默认材质改用同Font的Material80。向prefab外部表末尾追加sharedassets0引用，保留原有外部索引。

Font191原有 `m_DefaultMaterial=(0,80)`、`m_Texture=(0,147)`，Material80的_MainTex也指向Texture147，字体shader使用原有内置资源引用。Texture147是原字体动态atlas，初始0×0、空像素；候选复用整套原依赖，不新建材质或复制纹理。运行时生成中文字形及渲染兼容性尚需原机验证。

生成时重载检查37个对象库存、25个非目标对象原始字节、12个目标对象除字体/材质外全部字段、原有外部引用及新目标PPtr。TextMesh文字、颜色、字号、布局、Title和unionFlag对象保持。serialized文件因增加外部引用而重新生成元数据，对象偏移可变化。manifest记录全部对象输入SHA、字体依赖SHA、原DLL/dex SHA、源码SHA和精确PPtr变化。

这是字体缺口的候选修复。`<1>`来自独立区服行，姓名实收、运行时TextMesh文字和最终画面仍由主代理验证；静态成功不代表野外姓名故障已完全解决。


发布打包使用 `package.py`：只替换指定字体prefab和Android Manifest版本，保留正式69的所有DLL与DEX逐字相同。版本显示及更新上报沿用包内既有PackageManager读取，避免根目录旧版update构建工具造成回退。初始通用update候选已废弃且未安装。
