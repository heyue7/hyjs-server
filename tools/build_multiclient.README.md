# 双端原客户端

在仓库根目录运行，SHA 使用当前已审查的本地 APK 值：

```sh
rtk proxy python3 tools/build_multiclient.py --expected-source-sha256 1a38df6a4ae845a1f4ba64133038d25ef7489c0500fac502f2085b817b45b053
```

固定输入 `build/hyjs-local.apk`，固定输出 `build/multi-client/hyjs-multiclient.apk` 和小型校验证明 `proof.json`。包名为 `com.yunchang.djsy.ewan.multi`，标签为 `黑衣剑士·自建服·双端`。仅修改二进制 Manifest 中的包名和两个标签，保留其他 XML 节点及全部其他载荷的原压缩字节，并逐项核对。Assembly、语音桥接和原生组件沿用输入包；本工具不重新编译它们。

使用现有 `.local-signing/debug.keystore` 和本机 Android SDK 33.0.0，执行 zipalign、apksigner 及 aapt2 校验。输入和输出证书必须一致。独立构建锁与现有客户端构建锁保护输入；临时 APK 在成功和失败时均自动删除。不会覆盖当前或回滚包，不安装、不卸载、不连接设备，也不操作账号或数据库。实际双端收播仍由主线程安装后验收。
