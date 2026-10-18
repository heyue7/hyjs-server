# 最终客户端组合工具

`package_release.py`已由主代理完成构建、签名及继承条目校验。最终图标使用用户要求的“灵魂之花”新版；当前使用`branding-r3`，`branding-r1/r2`保留为历史证据。实际原机系统应用信息页已确认新版图标及名称，记录在`analysis/admin-wallet-polish-20261017/phone-r2/branding-installed.json`。

`branding-r3`从当前基线的`resources.arsc`解析`supersdk_icon`的真实资源ID，并按Android二进制XML属性资源ID顺序插入`android:icon`。`branding-r2`把该属性追加在末尾，静态解析可以找到图标，原机系统仍显示默认图标；这次实际失败保留在`phone-r1`，不能用静态解析结果替代原机确认。

root统一准备：

1. 将image最终图片按原256×256派生为`tools/client_branding/icon.png`；`branding.py`基于6074 APK导出新的`branding-r3`，不改原APK。
2. 固定本轮wallet UI build目录；其config、钱包icon、六资源刷新JSON必须与6074一致，只取新classes2.dex。
3. 复用`analysis/missing-config-branding-20261017/unavailable-ui-r1`，输入21e71669、输出e4f2503c。5个UI方法prefix与547Engine/六绿/轮首保持证据完整。
4. 在新record路径调用合成器。以下为后续复用示例，已有构建证据不得覆盖。

```sh
rtk proxy .analysis-venv/bin/python tools/client_branding/package_release.py \
  --base-apk build/hyjs-growth-candidate.apk \
  --base-proof analysis/admin-growth-20261017/apk-build-r4.json \
  --installed-proof analysis/green-hero-compat-20261017/phone-r2/final.json \
  --ui build/<本轮固定wallet目录> \
  --unavailable-overlay analysis/missing-config-branding-20261017/unavailable-ui-r1 \
  --branding-overlay analysis/missing-config-branding-20261017/branding-r3 \
  --record analysis/<本轮批次>/<新的APK构建记录>.json
```

合成器不要求先promote旧current。它只接受实际已安装6074候选的固定r4/phone-r2证据，逐项核qualifiedWalletEntries与签名身份；不会伪造promotion。共享`.client-build.lock`串行化打包。

仅允许四个现存entry变化：`classes2.dex`、`assets/bin/Data/Managed/Assembly-CSharp.dll`、`AndroidManifest.xml`、`res/drawable/supersdk_icon.png`。所有其他条目逐内容SHA保持；原compressed payload直接拷贝，正式zipalign/sign后再逐压缩SHA、压缩算法、压缩尺寸、原尺寸、CRC复核。不会把重压缩后相同解压字节冒称压缩数据不变。zipalign允许重写必要的对齐padding。

使用原common.run、apksigner与zipalign、现有debug.keystore，禁止生成新key。DLL守卫用其verify函数从6074输入重新计算全部差量与547Engine；品牌manifest也从6074原manifest重算，只允许应用名/启动页名与图标属性变化。

只产生新`build/hyjs-ops-wallet-candidate.apk`及新JSON证据；current/rollback逐SHA/存在性保持。已存在candidate或record时拒绝覆盖，临时unsigned/aligned/signed目录自动清理。proof继承qualifiedWalletEntries后更新四项，bridge原身份保持而outputSHA改新DLL；manifestProof使用新branding proof。安装与promote由root另行管理。
