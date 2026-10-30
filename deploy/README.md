# Ubuntu / Docker 部署

部署目标、对外 API、管理后台来源、端口和 SSH 用户由本机 `deploy/.env` 管理。
仓库只保留 `.env.example`；请勿提交真实地址、数据库密码、管理员凭据或存档。

## 准备配置

```bash
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
# 编辑 deploy/.env，填写自己的目标和端口
bash deploy_test.sh --check-config
```

`HYJS_DEPLOY_SSH` 为 SSH 目标；`HYJS_DEPLOY_ROOT` 必须是独占项目目录；
`HYJS_DEPLOY_UPLOADER` 为上传文件所属的远端系统用户。
`HYJS_PUBLIC_HOST` 为客户端可访问的主机名或 IP。
API、后台 origin 必须分别匹配 HTTP、后台端口，且不带路径或末尾斜杠。
HTTP、后台、TCP 与内部反向代理端口必须互不相同。

Go 的 `config.test.yaml` 和 Nginx 配置在构建 stage 中根据模板生成，
stage 和部署证据应由 Git 忽略。Compose 从远端私有 `.env` 插值；
首次部署生成数据库密码，后续部署仅更新拓扑变量，保留已有密码。
管理员凭据沿用已迁移数据库，构建物不包含管理员初始密码。

## 发布

需要 Python 3.11+、Go（由 go.mod 指定）、Node / pnpm、SSH / SCP 和 Bash。
本机还需已有的 `rtk` 命令代理。远端要求 Ubuntu、sudo 非交互权限、
Docker、Nginx；首次 provision 可安装 Compose 插件。

从干净的已检出 Git 提交执行：

```bash
bash deploy_test.sh --revision master
# 只更新已审核的本项目 Nginx 客户端路由，不重做首次基础设施
bash deploy_test.sh --revision master --update-ingress
# 首次创建独立目录、MySQL 与 Nginx ingress，并导入私有迁移快照
bash deploy_test.sh --revision master --provision --initial-dump /private/path/initial.sql.gz
# 可指定另一份本机配置
bash deploy_test.sh --env-file /private/path/deployment.env --revision test
```

首次数据包同目录需有 `initial-verification.json`，包含快照 SHA、各表数量和
完整 profile 校验摘要。仅允许对空项目数据库首次导入；重试相同快照不会重复导入。
迁移脚本 `prepare_initial.py` 会停止已核实归属的本机写入进程；执行前先明确切换范围。

脚本检查 CORS、运行 Go vet，编译 Linux/amd64（CGO 关闭，使用 MySQL），
并构建同一 Git 提交的管理前端。Compose 使用专属 MySQL 8.4 与游戏容器。
Nginx 管理入口为 `/admin/`，API ingress 仅开放客户端协议，保留私有运维路径隔离。
数据库、日志、私有环境与备份位于应用目录之外，不依赖 Redis。
已有环境更改 ingress 配置需显式使用 `--update-ingress`；首次配置使用 `--provision`。

## 回退与证据

在干净 checkout 中检出已记录提交，用同一入口的 `--revision <commit>` 发布。
数据库不会自动回退：保留首次快照及最近一次发布前快照，先检查 schema / 数据兼容性。
代码历史由 Git 保存，远端仅保留一个应用制品目录。

本机发布记录在 `analysis/deploy-20261018/`，远端在项目目录的 `deployed.json`。
这些记录含实际来源地址，应保持本机私有。脚本仅管理带归属标记的项目资源，
不管理其他容器、站点或数据库。

## Brain Burst 2039 角色钱包切换

首次切换使用 `--migrate-role-wallets --expected-live-revision <完整当前提交>`。
脚本先校验唯一旧角色和现网提交，在服务仍运行时构建镜像，随后停旧应用，
固定旧账摘要并完成加密备份及解密验证，再启动新应用执行事务迁移。
MySQL 保持运行；核对旧表摘要、全部历史字段、迁移标记和角色余额/revision后才记录发布成功。
核账失败会停应用并保留备份及证据。后续正常发布无需首次迁移参数。

迁移保留旧表供核对。角色钱包开始产生交易后，旧二进制读取的旧账号余额已冻结，
不能直接回退运行。恢复时先停写，保全新角色流水，核清迁移后交易，
再选择修复当前角色钱包版本，或在完整核账后恢复业务备份与对应 Git 版本。
本批不附带旧钱包兼容二进制。

## 安全基础（批28）

- 本批按用户决定保留现有HTTP，不启用HTTPS。后台Cookie保留HttpOnly/SameSite=Strict；如以后启用TLS，再显式配置HYJS_ADMIN_SECURE_COOKIES。不能把X-Forwarded-Proto当作任意客户端可信输入。
- activate.py只把本部署Docker网桥的实际gateway加入HYJS_TRUSTED_PROXIES；Nginx覆盖传入的X-Forwarded-For，防止客户端伪造来源。不要设置任意来源可信。
- 新部署前备份采用HYJSBK02分块格式AES-256-GCM，密钥保存在部署根的.backup-encryption.key（0600），备份在私有backups目录（0700）。密钥与备份分别保管，密钥不进入Git、日志、制品包或数据库。历史备份保留原权限，不自动清理业务历史。
- 加密后先解密并核对摘要，再删除本次临时明文；失败保留私有.pending文件，部署中止。当前工具按4MiB分块认证，单快照上限8GiB；仍兼容历史HYJSBK01读取。达到上限或认证失败时中止，禁止跳过备份。
- 手工恢复前先用hyjs-backupseal --decrypt --key-file 私有密钥路径 --input 选定.aesgcm --output 新的私有.sql.gz进行认证解密；该命令不执行数据库恢复。必须确认目标实例与所选备份后才导入。

### 回退约束

当前制品包含现行业务程序、备份认证工具和86存档维护工具，不附带旧版兼容回退程序。历史源码保存在Git；旧部署中的回退Compose覆盖示例已失效，不能套用于当前镜像。

回退前保持维护停写，认证备份并保全新流水、回执及存档；优先修复当前兼容版本。86的角色库存压缩和多线首领状态必须分别核兼容：`hyjs-profile-storage86` 的有界展开只处理库存编码，不能证明95a7能够读取多线首领或安全直接回切。维护工具及限制见 `analysis/desert-final86/profile-rollback86.md`，执行时须独立确认应用已停止。

### 维护期重部署兼容版本

对已经存在的环境，可在核验候选程序兼容现有 schema、压缩存档、多线首领、在途战斗和账本，并完成隔离演练后，使用干净固定提交执行：

```bash
rtk proxy bash deploy_test.sh --revision <已验证的兼容候选提交> --expected-live-revision <完整当前提交> --maintenance-redeploy
```

该模式复用 82/84/85 的维护守卫：核当前部署记录、现有应用容器的版本标签、维护状态和既有数据库，停止应用，生成并认证本轮加密备份，校验备份身份和维护，再按原入口启动候选并核健康；健康后复查维护并记录 `maintenanceRedeployed`、候选摘要和私密 manifest。容器不存在或版本标签与记录不符时拒绝执行，需先核对中断现场。模式中不执行 82/84/85 或其他配置投放命令；禁止与任何投放、首次 provision/import、钱包迁移、指定玩家预热、日历恢复和 ingress 修改混用。维护不会自动解除。

调用前仍须独立确认所有应用实例、工作器和其他写入者均已停止，原事务已收口，维护期间阻止其他操作者解除维护，并核实认证备份的恢复可行性。脚本停止本 Compose 的 `app`，不发现或停止其他写入者，维护状态复查也无法排除两次检查之间被改动的情况。`--warm-rush-history-all` 仍会在备份后执行有界历史证明更新；候选启动的初始化、迁移和后台恢复任务也可能写入。健康启动后的新增流水、回执及存档须保留，不能自动用备份覆盖。

维护断线可能触发撤离取消或完成、交易取消、采集资格变化；Boss 和觉醒探索仍按真实时间推进。须在隔离环境验证对应回执和资产，不把停服当成全部在途状态暂停。异常、中断或健康失败时守卫尝试停应用并保留证据，若停止失败须人工核实；SIGKILL、宿主机断电及部分写入已提交时，恢复前核对实际容器、镜像、维护状态、manifest 和数据库。此模式不保证旧程序兼容，不自动恢复旧数据库。`GGOExtractionReady=false` 仍会使已启用沙漠部署的野外快照失败；隔离演练须验证候选与当前发布配置的完整恢复链。

## 75版维护投放

从审核通过的干净提交运行原 `deploy.py --revision <commit> --expected-live-revision <live-commit> --apply-growth75`，需要更新入口规则时另外明确传 `--update-ingress`。禁止与74投放、首次导库或角色钱包迁移混用。正常启动不执行75投放。

部署保留维护并停应用，生成并认证本轮加密数据库备份；候选程序 `--apply-growth75 --server-id 1` 先核对维护、audit321及完整820条V12排期，再依次恢复真实发布时间、修复日历锚点、应用经济目录、发布1.0.75/build2026102702兼容门槛。完成后仍处于维护。

候选CLI成功之后才新鲜导出活动原数据；用同一制品中的 `growth75_activity_patch.py` 生成带原值CAS的SQL，在一个utf8mb4 MySQL连接内执行。私有 `backups/growth75-*/manifest.json` 记录本轮加密备份、快照/SQL摘要、修改条数和结果。任一CAS失败时，未提交事务随连接关闭回滚，应用保持停止；此前CLI已提交的领域步骤保留审计，可核对证据后重试。脚本不会自动恢复整个数据库或自动开服。维护服务恢复运行仅用于健康检查，开服由后台人工执行。

本地脚本编排检查：`python3 -m unittest discover -s deploy -p test_growth75_activation.py`，不连接SSH、Docker或数据库。实际区服核账和原协议验收另行执行。

## 试炼 82、夺宝 84 与养成 85 维护投放

从已审核的干净提交执行现有部署入口，指定完整的当前线上提交：

```bash
rtk proxy bash deploy_test.sh --revision <候选提交> --expected-live-revision <完整当前提交> --apply-trial82
# 仅投放夺宝扫荡开关
rtk proxy bash deploy_test.sh --revision <候选提交> --expected-live-revision <完整当前提交> --apply-feedback84
# 同一维护窗口顺序投放 82、84 与 85，并更新已审核的现有 ingress
rtk proxy bash deploy_test.sh --revision <候选提交> --expected-live-revision <完整当前提交> --apply-trial82 --apply-feedback84 --apply-feedback85 --update-ingress
```

`--apply-trial82`、`--apply-feedback84`、`--apply-feedback85` 可分别选择或共同选择，均只允许已有环境，禁止与首次 `--provision`、初始导库、钱包迁移、显式 `--warm-rush-history-player`、日历恢复及其他投放参数混用。必须指定完整且与远端部署记录一致的当前版本。既有 `--warm-rush-history-all` 发布门禁仍会执行：候选程序不监听网络，按有界事务重验过期派生证明，不改资源、进度和回执。`--update-ingress` 可与本次投放同批使用；远端要求已有本项目站点配置及其启用链接，检查 Nginx 后只在配置变更时重载。该参数不执行首次基础设施或入口创建。

脚本先建立私密投放 manifest 并检查维护状态，停应用后完成一次加密备份和认证校验；同时选择时依次执行候选程序 `--apply-trial82`、`--apply-feedback84`、`--apply-feedback85`，每项完成后复核维护状态。命令分别校验既有管理员、时钟、维护及其配置约束，配置漂移会拒绝投放。manifest 保存候选程序与备份摘要、目标提交、实际请求的命令、逐命令结果和失败阶段；成功的部署记录分别写入 `trial82Applied`、`feedback84Applied`、`feedback85Applied`，指向同一份投放证明。

备份校验、候选命令、启动、健康检查或中断失败时，守卫保持应用停止并记录失败；若停止命令失败会明确报错。82 成功后 84 失败时保留 82 的已审计结果、备份和证据，核查后按各命令原有的幂等及漂移检查重试；脚本不自动恢复数据库或解除维护。成功记录部署前也会复核维护状态，正常启动不自动执行上述投放。SIGHUP、SIGTERM 和键盘中断可触发进程内守卫；SIGKILL 或宿主机断电无法执行该守卫，恢复前需检查服务状态和私密 manifest。

本地编排检查：`rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy -p test_trial82_activation.py`。该检查使用本地夹具，不连接 SSH、Docker 或数据库；实际发布和区服验收另行完成。

## 86 存档格式维护工具

同提交制品附带 `hyjs-profile-storage86`，摘要纳入 `artifact.json`。默认只读预览；具体参数与停写、备份认证、维护、限批和中断续接要求见 `analysis/desert-final86/profile-rollback86.md` 与 CLI `--help`。该工具只把压缩 warriors 无损展开为旧 JSON 数组，不改变角色其余字段和业务历史。执行前必须实际停止应用、认证并固定最近备份、核目标数据库和数据库触发器；`--app-stopped` 仅记录操作者已完成前提，不检测进程。旧版对多线 Boss 和新业务状态的兼容仍需单独验证，不能展开存档后直接盲切 95a7fb6。
