# 项目磁盘只读审查（2026-10-25）

审查按 hyjs-development、RTK 与本机共享服务规则执行；没有删除、压缩、运行构建、连接数据库、启动或停止资源。仅写本报告及 disk-candidates.json。当前后台活动工作树未操作。

## 可执行候选

| 绝对路径 | 实际大小 | 生成来源与引用 | 判定 |
| --- | ---: | --- | --- |
| /Users/lirui/LR/hyjs/apks/build/hyjs-brain-burst-2039-candidate.apk | 683,640,057 B（652 MiB） | plans/20261025-brain-burst-2039.md记录早期账号候选，未安装/晋升。SHA43b09115…a87；nlink=1。67正式r5已取代；当前67 overlay明确基于66，未引用此候选 | **可删除**，保留旁边源码、manifest及历史证据 |
| /Users/lirui/LR/hyjs/build/hyjs-server | 58,255,890 B（55.6 MiB） | 旧本机Go编译产物，SHA6fbda836…0cc；与67线上SHA12e0e925…不同，无本机进程/容器引用。deploy/deploy.py自行从源码构建到stage | **有条件可删除**：主代理确认不承担必要本地回退。源码/发布清单保留 |

明确候选可回收约652 MiB；加入条件项约708 MiB。上表文件均未被Git跟踪。执行时须再核SHA、inode、进程和当前发布状态，避免本报告产生后其他任务改写同路径。

## 必须保留与不确定项

| 绝对路径 | 大小 | 依据 |
| --- | ---: | --- |
| /Users/lirui/LR/hyjs/build/hyjs-local.apk；/Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.67.apk；/Users/lirui/.codex/worktrees/brain-burst-release/hyjs/build/hyjs-brain-burst67-r5.apk | 684,713,465 B，三路径同inode292295907 | 当前正式67，完整SHA1408da34…06b。硬链接只计一份占用；删一个别名不会回收全包 |
| /Users/lirui/LR/hyjs/build/hyjs-rollback.apk；/Users/lirui/LR/hyjs/build/hyjs-feedback66-formal-r7.apk；/Users/lirui/.codex/worktrees/feedback63-field-trial-copy/hyjs/build/hyjs-local.apk；同目录hyjs-feedback66-formal-r7.apk | 683,709,864 B，四路径同inode292142899 | 66直接客户端回退及67固定输入。67/client-overlay-r5/manifest.json指定base SHA60be2002…5b4。全部保留 |
| /Users/lirui/.codex/worktrees/feedback63-field-trial-copy/hyjs/build/hyjs-feedback65-formal-r4.apk | 683,701,497 B | client_feedback66/build.py固定BASE输入。不能按旧版本号删除 |
| /Users/lirui/.codex/worktrees/feedback63-field-trial-copy/hyjs/build/hyjs-feedback64-formal-r2.apk | 683,697,312 B | client_feedback65/README.md命令明确输入该包，build.py校固定BASE。保留构建链 |
| /Users/lirui/LR/hyjs/apks/build/hyjs-local.apk；/Users/lirui/.codex/worktrees/ggo-items-shop/hyjs/build/hyjs-local.apk；同目录hyjs-latency62-r4.apk | 683,676,688 B，三路径同inode291131369 | SHAfe427f13…580；为旧62命名链。主build current已67，不能误认成当前正式包。历史构建链输入尚未完全排除，保留 |
| /Users/lirui/LR/hyjs/build/hyjs-formal51-rollback.apk；/Users/lirui/.codex/worktrees/move-confirm49/hyjs/build/hyjs-instance-lock51-final-v1.0.51.apk | 668,978,564 B，同inode两链接 | 52计划记载直接回退；desert delivery-status.json也固定其SHA46429701…856为sourceAPK。保留 |
| /Users/lirui/LR/hyjs/build/hyjs-formal49-rollback.apk；/Users/lirui/.codex/worktrees/move-confirm49/hyjs/build/hyjs-feedback49-v1.0.49.apk | 668,904,744 B，同inode两链接 | 历史固定回退/输入未排除，保留 |
| /Users/lirui/LR/hyjs/build/desert-unity53/hyjs-desert-walk-qa-r10.apk | 682,974,838 B | delivery-status.json记录最新已原机验证r10，场景仍未正式集成且须重新绑定最新正式APK。属于该未完成功能的可复用候选，保留 |
| /Users/lirui/LR/hyjs/build/desert-unity53/hyjs-desert-walk-qa-server | 61,108,418 B | tools/desert_unity53/qa_server.py依赖固定二进制并校SHA；delivery状态明确QA服务停机、资源释放，后续可直接复用，不先删 |
| /Users/lirui/LR/hyjs/build/desert-unity53/qa-server-source | 约24.6 MiB | qa_server.py逐源码/table pin验证，构建输入；禁止整目录删除 |
| /Users/lirui/LR/hyjs/build/unity-desert-reconstruction-20261021 | 约213 MiB | reference-built-29含生成Assets约86MiB、Packages约17MiB，reference-revision/离线包/交换输入仍在。源码、原资源、审查截图、场景交付混合，未发现可直接整体清的Library/Temp缓存，保留 |
| /Users/lirui/LR/hyjs/build/player-terminal-adjust-20261022 | 约76 MiB | git worktree list注册的真实工作树，不能按build子目录当临时产物删除 |
| /Users/lirui/LR/hyjs/apks/original | 约4.0 GiB | 原APK及原始提取证据；明确保护 |
| /Users/lirui/LR/hyjs/admin-web/node_modules | 约803 MiB | 多工作树通过symlink共享，当前后台任务需要，保留；worktree下du=0仅是符号链接 |

**59固定输入缺口**：plans/61-recycle100-20261024.md写明应保留 `build/hyjs-feedback59-r4.apk`（SHA957ca6dd…265b）。本次针对主项目与所有 `/Users/lirui/.codex/worktrees` 的真实 `*.apk` 枚举未发现59命名文件。不能宣称固定59包已核验保留；应由主代理核对历史cleanup/是否另有外置归档。此次没有删除任何文件。

## 体积组成及不可整清原因

- /Users/lirui/LR/hyjs/analysis约11.8GiB。p6约3.8GiB中，JSON约3,010MiB、JSONL约881MiB、gz约501MiB，均属于协议/战斗历史证据或审查输入；JSON、JSONL及其压缩副本用途未逐项核实，全部保留。
- /Users/lirui/LR/hyjs/analysis/device约1.57GiB，其中PNG约1,522MiB、JPG约69MiB；全部截图证据，保留。
- /Users/lirui/LR/hyjs/analysis/growth-first-20261016约1.7GiB，PNG约1,309MiB、JSONL约254MiB；/Users/lirui/LR/hyjs/analysis/p15约676MiB，PNG约648MiB。不能当缓存整清。
- 编译缓存定向检查：上述Unity工程和旧构建目录未发现大型Library、Temp、obj、bin或.gradle缓存。brain-burst-release与feedback63工作树仅有零散Python __pycache__，总量不足2MiB，收益很小；不扩大清理到共享全局Go/Gradle缓存。
- 当前67、66、62、51、49均有硬链接，表面路径大小相加会高估释放量。物理占用以inode和最后链接删除后结果为准。

## 运行/停止资源引用

只读 `docker ps -a` 与所有四个容器mount检查：codex-local-mysql、codex-local-redis在运行，两套其他项目minio停止。没有任何容器挂载HYJS项目构建目录；共享数据库卷、其他项目卷均不属于候选。未做Docker清理。

只读 `ps`：未见Unity、Java/Gradle、Mono/dotnet、hyjs服务进程；adb在运行。定向lsof仅见adb当前目录为项目根，无APK/旧二进制打开引用。未操作手机或adb。删除执行前应重查，后台代理仍在活动。

所有注册工作树及其源码/历史保持；本次不建议归档工作树。当前 `/Users/lirui/.codex/worktrees/admin-ui-polish-release/hyjs` 由其他代理使用，未操作。
