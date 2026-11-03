# 1.1.8 沙漠 Boss 奖励显示增量构建

该工具读取固定 SHA-256 的正式 1.1.8 APK，生成两个托管程序集及 `manifest.json`，供现有 `tools/client_release/package_native.py` 读取。工具不打包 APK、不安装设备、不发布。

在本工作树中执行：

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 .analysis-venv/bin/python tools/client_rank_desert_patch/build.py
```

默认输出目录为 `build/rank-desert-fixes-20261103/client`，必须事先不存在。`--output-directory` 可指定项目内的新目录；`--base-apk` 和 `--runtime` 可指定输入文件及本地 SDK，APK SHA-256 仍固定。运行需要本地 `dnfile`、`dncil`，以及指定 runtime 内已有的 .NET 8 SDK、参考程序集和 dnlib；编译过程不执行 restore 或下载。

构建先从 Git 提交 `b0967e83a2327feab5c4510c13f040ba06dd8835` 提取旧版 `SOURCES` 和三份 recovery 源码，以发布包中的引用和原图资源重编译旧助手；必须与 APK 内的 `SoulFlowerDesertV4.dll` 逐字节相等。当前助手只允许 `BattleRecovery.cs` 发生源码变化，并允许 `SOURCES` 新增 `BossRewardView.cs`。其余参与编译的源码及图资源必须等于该提交。源码快照及最终文件哈希会记录在清单中；发现源码或构建工具在构建期间变化时直接失败。

主程序集仅在 `Battle.CopyBalance::Refresh` 的 Boss 奖励结果路径中插入一次现有 `FieldHooks.MonsterRewardDisplay` MemberRef 调用。构建器核验两条奖励展示路径、方法与元数据表、Unity 兼容的 CLI 标志及指针表，逐条比对原方法指令、跳转目标、异常边界、locals 和 maxstack，并要求其他所有原方法体及 547 个 `Battle.Engine` 方法保持原字节。助手程序集的原有公开 API 与嵌入资源也须一致。

输出 `Assembly-CSharp.dll`、`SoulFlowerDesertV4.dll` 和 `manifest.json`。清单中的静态验证状态仅覆盖构建检查；客户端实际运行和设备验收需要另行完成。
