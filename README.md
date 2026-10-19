# 灵魂之花

灵魂之花游戏服务端与运营后台。服务端使用 Go 重建账号、存档和游戏业务，保留客户端原有 HTTP/TCP 二进制 Protobuf 协议；后台使用 Vue 3 管理角色、道具和运营配置。

本项目仍在完善。部分后段地图和玩法尚未开放，部分规则使用已标注的兼容实现；功能代码、资源配置和实际验收范围分别维护，不以目录存在表示完整还原。

## 功能范围

- 全局账号注册、登录、昵称及密码管理，各区服独立角色存档与新手引导协议。
- 人物、装备饰品、精灵、宝石、料理及相关养成结算。
- 主线、副本、试炼、精英、远征和已恢复的战斗结算。
- 抽卡、商店、转盘、活动、礼包与会员配置。
- 蓝绿钱包：测试余额、充值订单、收支账本和账单。
- 运营后台：全局用户、区服玩家、道具、活动、各类商店、成就、称号、公告和客服反馈。
- 新玩家相对排期与固定日期排期，开服任务按区服自然日解锁、保留每个角色完整参与时长。
- 野外整组资源方案及区服分线管理，缺失场景保持暂未开放。

蓝绿钱包用于本项目内的模拟体验，不接入实际收款渠道。未恢复的资源与运营投放仍需单独配置。

## 项目结构

| 目录 | 内容 |
| --- | --- |
| `hyjs-server/` | Go 服务、数据库模型、协议处理与业务逻辑 |
| `admin-web/` | Vue 3 / TypeScript 运营后台，基于 Soybean Admin |
| `server/protocol/` | 客户端协议定义及辅助实现 |
| `analysis/tables/` | 服务端读取的游戏配置表 |
| `tools/` | 构建、协议分析、迁移及客户端适配工具 |
| `tests/` | 协议、客户端与隔离环境验证 |
| `deploy/` | Ubuntu / Docker Compose / Nginx 发布脚本与模板 |
| `apks/` | 本机 APK 输入和产物，Git 忽略 |

## 环境要求

- Go：以 `hyjs-server/go.mod` 为准，当前为 **1.27.2**。
- MySQL **8.4**。主服务使用 MySQL；SQLite 仅用于显式选择的隔离测试与迁移兼容。
- 后台：Node.js **20.19+ 或 22.12+**、pnpm **10+**，具体约束见 `admin-web/package.json`。
- Python **3.11+**：仅辅助工具和发布脚本需要，运行 Go 二进制无需 Python。
- Docker / Docker Compose：使用容器部署时需要。游戏主链不依赖 Redis。

客户端适配工具另外需要合法持有的 APK、本机 Android SDK、Java 及相应 Python/.NET 依赖；仓库不提供 APK 和签名密钥。部分内部辅助脚本使用 `rtk` 命令代理及本地共享服务工具，普通 Go 启动可直接按下面步骤进行。

## 配置与启动

仓库只提交配置模板。实际服务器地址、数据库密码、管理员凭据和运行配置保存在被忽略的文件中。

```bash
git clone git@github.com:heyue7/hyjs-server.git
cd hyjs-server
cp .env.example .env
cp hyjs-server/config.example.yaml hyjs-server/config.dev.yaml
```

编辑 `.env`，填写自己的数据库连接和客户端可访问地址。先创建专属数据库和用户，再启动服务。监听地址决定服务绑定的网络接口；`HYJS_GAME_ADVERTISE_HOST` 是原协议返回给客户端的主机名或 IP，手机需要能够访问它及 HTTP/TCP 两个端口。

```bash
set -a
. ./.env
set +a
cd hyjs-server
go mod download
go run . --env dev
```

配置优先级为：**命令行覆盖参数 > 进程环境变量 > 私有 YAML**。Go 读取 `config.<env>.yaml`，缺失时回退 `config.yaml`；也可用 `HYJS_CONFIG_FILE` 指定私有配置文件。指定文件不存在时启动失败。Go 不自动解析 `.env`，上例由 shell 加载，也可交给进程管理器或 Compose 注入。

| 配置 | 用途 |
| --- | --- |
| `HYJS_MYSQL_HOST / USER / PASSWORD / DATABASE` | MySQL 连接 |
| `HYJS_GAME_ACCOUNTS_DATABASE` | 同一 MySQL 实例的共享账号/钱包库，可选，默认当前角色库 |
| `HYJS_GAME_BIND` | 监听接口 |
| `HYJS_GAME_ADVERTISE_HOST` | 原协议公布给客户端的地址 |
| `HYJS_GAME_HTTP_PORT / TCP_PORT` | 游戏 HTTP / TCP 端口 |
| `HYJS_GAME_TABLES` | 游戏配置表目录 |
| `HYJS_CORS_ORIGINS` | 精确允许的浏览器来源，以逗号分隔 |
| `HYJS_ADMIN_ASSETS` | 编译后的后台静态目录 |
| `HYJS_ADMIN_BOOTSTRAP_FILE` | 首次建立管理员所用的私有 JSON 文件 |

共享账号库须事先创建，并给各区服数据库用户授予所需权限；各区服角色库仍独立。该选项要求共享库与角色库位于同一 MySQL 实例，以保持钱包扣款和充值结算的事务一致性。

更多配置见 [Go 配置模板](hyjs-server/config.example.yaml) 和 [配置加载实现](hyjs-server/config/config.go)。默认没有公开的管理员密码。首次初始化使用的 JSON 格式为 `{"username":"your-admin","password":"your-own-strong-password"}`，文件权限必须为 `0600`；通过 `HYJS_ADMIN_BOOTSTRAP_FILE` 指定其绝对路径。已有管理员时不会覆盖其账号。

启动后可用 `/health` 检查进程与数据库；该接口不能代替完整玩法验收。客户端协议入口为 `/sao/`，后台入口为 `/admin/`。

## 运营后台

```bash
cd admin-web
cp .env.example .env
cp .env.prod.example .env.prod
cp .env.test.example .env.test
pnpm install --frozen-lockfile
pnpm build
```

把 `HYJS_ADMIN_ASSETS` 指向 `admin-web/dist` 的绝对路径，再启动 Go 服务。后台默认使用同源 `/admin/api`，无需在前端源码中填写服务器 IP。开发模式可用 `pnpm dev`；代理和基础路径通过本机 `.env` 配置。

后台环境变量会进入浏览器构建物，仅用于公开的页面参数。数据库密码、签名密钥及管理员密码应只放在服务端配置中。

## 客户端与 APK

本机文件按以下约定整理，全部不进入 Git：

```text
apks/
  original/    # 原 APK、GM 参考 APK
  build/       # 自建 APK、当前版本和必要回退包
```

`build/` 中可保留兼容旧脚本的软链接。客户端 API 地址由 `tools/client_remote/.env` 的 `HYJS_API_ORIGIN` 管理，模板为 [客户端环境示例](tools/client_remote/.env.example)。也支持显式 `--origin`；服务端公布的 TCP 地址仍由 Go 配置管理。

客户端工具会校验原始输入和候选证明，依赖相应本机分析产物。它们用于适配指定版本，不代表任意 APK 都能直接构建。

## 验证与构建

```bash
cd hyjs-server
go test ./config ./middleware
go vet ./...
go build -trimpath -o ../build/hyjs-server .
```

涉及数据库交易和玩法的验证按对应隔离环境运行；不将测试指向运行中的角色存档。发布脚本还会校验 Linux 构建、后台构建和制品摘要。验证记录、账号夹具与手机截图保留在本地。

## 部署

部署流程见 [部署说明](deploy/README.md)。从 `deploy/.env.example` 创建私有 `deploy/.env`，执行配置预检，再按固定的干净 Git 提交发布：

```bash
bash deploy_test.sh --check-config
bash deploy_test.sh --revision master
```

远端使用独立 MySQL 与游戏容器，Nginx 提供后台和 API 入口。数据库、日志、环境文件和备份保留在应用制品目录之外。代码回退通过 Git 与同一发布脚本完成，数据库需单独处理恢复。

## 分支与本机文件

- `master`：可交付基线。
- `test`：联调与验证。
- `dev`：日常开发。

APK、实际 `.env` / Go YAML、数据库、备份、密钥、账号记录及内部研发计划均由 Git 忽略。提交前检查暂存区，避免把运行日志或私有配置加入版本库。

后台前端沿用 [Soybean Admin](https://github.com/soybeanjs/soybean-admin) 的组件与基础设计，其许可见 [admin-web/LICENSE](admin-web/LICENSE)。
