# 觉醒 v85 客户端布局

**当前状态：源码静态核对完成，待 C# 重建和测试。** `build80-r3`、`build83-r2` 及对应的 `scenes80-r4.json`、`scenes83-r4.json` 是本次夜林内存兼容修改前的历史对照，不可作为当前源码的最终产物。

此工具只生成 `HYJS.DungeonContent.dll` 和 `smallassetmanifest` 覆盖文件，不修改 Main、不打 APK。服务端 v85 条目必须使用 `p7-awake85-` 加 32 位小写十六进制作为 run ID，四箱依次使用 `:b1` 至 `:b4`，怪使用 `:m1`；已开的箱仍返回，击败的怪可以省略。四箱模板使用 `(+/-14,0,+/-14)` 的四个不同角，怪模板使用 `(0,0,0)`。布局仅在场景类型 10、章节 1–5 和完整同 run 标记同时成立时启用。

客户端从已加载 NavMesh 选择六个固定槽：出生、怪、箱 1–4。怪被击败时仍保留怪槽，箱按 GUID 后缀定位。点位须从出生位置有完整路径，出生和每个箱与怪的水平距离须大于 9；本项目选五种原怪的视距均为 5、游走半径均为 2，合计均为 7。旧条目沿用原逻辑。离开当前条目时，在识别下一条目前恢复共享章节原出生点。

`smallassetmanifest` 只补 `map/map_nightforestfield_001` 的既有资源条目。完全相同的条目幂等，重复、冲突、资源哈希或大小不符时拒绝处理。原有行与顺序保持不变。

旧安装优先读取设备持久清单。`AwakeAdapt` 在绑定 v85 章 1 条目后、场景解析前，通过原生 `AssetFileTool.LoadFromString` 解析同一夜林行；两个内存字典均校验后只添加缺失键。已有同值保持，任何冲突、空目录或解析失败会中止该入口；共享字典只操作一次。设备文件和其他索引保持原样。`tools/client_awake85_layout/upgrade_static_audit.py` 输出的静态报告列出来源散列和 80/83 依赖图结果。

## 构建

在项目根目录运行，输出目录必须是 `analysis/fragment-box85/awake-copy-entry/client/` 下尚不存在的目录。需要现有 `.analysis-venv`（`dnfile`、`UnityPy`）和 `.tools/native-batch`（.NET 8、Roslyn、dnlib）。

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_awake85_layout/build.py \
  --base-apk '/Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk' \
  --base-sha256 25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a \
  --expected-helper-sha256 71d6ee0966e0d656224027a402dc6c21a70cf2bb580a352278020580b59b327b \
  --tool-root /Users/lirui/LR/hyjs/.tools/native-batch \
  --output-directory analysis/fragment-box85/awake-copy-entry/client/新构建目录
```

可选的 `--input-managed` 和 `--input-managed-sha256` 必须成对指定，用来验证实际使用的 Main。工具检查输入包、helper、夜林资源、12 个现有 hook、547 个 Engine 方法，比较原 helper 的类型/字段/方法 ABI 和 31 个未修改方法的解析后 IL；报告列出输入、源码和输出散列。`scene_audit.py` 进一步核对项目五章表、资源依赖、已登记别名及可选 Desert53 加载器。`test.py` 编译运行真实的纯 C#3 契约、槽位、状态恢复和 manifest 输入测试。`baseline.py` 从固定 Git 提交重建原 helper，用于复核迁移前源码与 APK 的一致性。

后续获得构建授权后，可在新目录运行纯契约测试，再按上面的构建命令生成新覆盖，并针对各新构建运行 `scene_audit.py`：

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_awake85_layout/test.py \
  --base-apk '/Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk' \
  --base-sha256 25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a \
  --tool-root /Users/lirui/LR/hyjs/.tools/native-batch \
  --output-directory analysis/fragment-box85/awake-copy-entry/client/contract-night-r1
```

83 r3 的重建固定了原 QA 包和实际静态 Main：

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_awake85_layout/build.py \
  --base-apk /Users/lirui/.codex/worktrees/desert-extraction-v4/hyjs/build/desert-extraction-v4/desert-trial-qa-v83-r3.apk \
  --base-sha256 1d4b6798ffe54f2545a7d14b9109ae9bd84e7a6d66f7a17b253e213b414adaa8 \
  --expected-helper-sha256 71d6ee0966e0d656224027a402dc6c21a70cf2bb580a352278020580b59b327b \
  --input-managed /Users/lirui/.codex/worktrees/desert-extraction-v4/hyjs/analysis/desert-extraction-v4-client/combined83-r3/static/Assembly-CSharp.dll \
  --input-managed-sha256 54c0f8567c711d661b2743e8a48044c3c406f3ae8b80e03534d2794f52c46425 \
  --tool-root /Users/lirui/LR/hyjs/.tools/native-batch \
  --output-directory analysis/fragment-box85/awake-copy-entry/client/build83-night-r1
```

80 构建使用上面的通用命令，将输出目录指定为 `analysis/fragment-box85/awake-copy-entry/client/build80-night-r1`。两套新构建成功后，可按实际输入再次审计资源：

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_awake85_layout/scene_audit.py \
  --base-apk '/Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk' \
  --base-sha256 25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a \
  --overlay-directory analysis/fragment-box85/awake-copy-entry/client/build80-night-r1 \
  --output analysis/fragment-box85/awake-copy-entry/client/scenes80-night-r1.json

rtk proxy env PYTHONDONTWRITEBYTECODE=1 /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_awake85_layout/scene_audit.py \
  --base-apk /Users/lirui/.codex/worktrees/desert-extraction-v4/hyjs/build/desert-extraction-v4/desert-trial-qa-v83-r3.apk \
  --base-sha256 1d4b6798ffe54f2545a7d14b9109ae9bd84e7a6d66f7a17b253e213b414adaa8 \
  --input-managed /Users/lirui/.codex/worktrees/desert-extraction-v4/hyjs/analysis/desert-extraction-v4-client/combined83-r3/static/Assembly-CSharp.dll \
  --input-managed-sha256 54c0f8567c711d661b2743e8a48044c3c406f3ae8b80e03534d2794f52c46425 \
  --expected-desert-helper-sha256 f46592c7bd5ffcb000ab73ff6d5de9389315d1fc9b95d0ad04de7f534dda93a9 \
  --overlay-directory analysis/fragment-box85/awake-copy-entry/client/build83-night-r1 \
  --output analysis/fragment-box85/awake-copy-entry/client/scenes83-night-r1.json
```

## 运行边界

静态审计证明资源声明及指定小目录下的解析路径；设备上的持久资源目录、Unity 场景加载、NavMesh、角色素材、对战和冷重启均需另行验收。80 和 83 包内的 `android` 图都声明夜林根且该根无直接依赖；固定旧缓存满足原 resolver 的身份、路径和稳定性条件时会转向包内图。未知缓存可能保留另一张依赖图，仍需验收。布局最多尝试 6、8、10、12 个候选点；不存在满足要求的槽位时抛错，可能发生在服务端已接受入场后。上线前应逐章验证成功进入和首次加载耗时，并验证击败怪、已开箱、跨 run、旧 501 续局及返回战斗后的点位。
