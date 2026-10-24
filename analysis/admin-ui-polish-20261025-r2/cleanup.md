# 磁盘清理结果

已删除880250144B，约839MiB。

- /Users/lirui/LR/hyjs/apks/build/hyjs-brain-burst-2039-candidate.apk：683640057B；obsolete unreferenced reproducible candidate/cache
- /Users/lirui/LR/hyjs/build/hyjs-server：58255890B；obsolete unreferenced reproducible candidate/cache
- /Users/lirui/LR/hyjs/.local-env/admin-ui-r2：69056231B；owned isolated QA binary, SQLite fixtures and credentials; processes stopped
- /Users/lirui/LR/hyjs/analysis/admin-ui-polish-20261025/admin-dist：34645399B；superseded first-round reproducible assets; paired screenshots retained
- /Users/lirui/.codex/worktrees/admin-ui-polish-release/hyjs/admin-web/dist：34652567B；duplicate reproducible build; exact current delivery retained in r2/admin-dist

删除前SHA/单链接/进程/容器引用已核，当前67与66回退包保持；原APK、固定输入、业务及审查历史和共享资源保留。临时浏览器、QA库/凭据、端口与构建中间件已清。详细风险边界（含59既有缺口）见disk-audit.md。
