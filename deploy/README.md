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
更改 ingress 配置需显式使用 `--provision`。

## 回退与证据

在干净 checkout 中检出已记录提交，用同一入口的 `--revision <commit>` 发布。
数据库不会自动回退：保留首次快照及最近一次发布前快照，先检查 schema / 数据兼容性。
代码历史由 Git 保存，远端仅保留一个应用制品目录。

本机发布记录在 `analysis/deploy-20261018/`，远端在项目目录的 `deployed.json`。
这些记录含实际来源地址，应保持本机私有。脚本仅管理带归属标记的项目资源，
不管理其他容器、站点或数据库。
