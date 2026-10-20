# AGENTS.md

## 项目约定

- 这是一个 Gin + Gorm 的 Go 服务，入口在 `main.go`。
- 代码变更后先执行 `gofmt`，开发中运行受影响测试；相互依赖的差量合成一批后统一完成全包编译、`go vet` 和构建。完整 `go test ./...` 在共享核心广泛变更、影响不明、关键里程碑及最终交付执行；已有可信同源基线时可复用全量结果并验证增量，不能声称本批已全量重跑。具体采用根目录 [hyjs-development](../skills/hyjs-development/SKILL.md) 的验证分层。
- 依赖整理使用 `go mod tidy`，不要手工编辑 `go.sum`。
- Shell 命令遵循本机 Codex 约定，优先使用 `rtk` 前缀。
- 同时定义大于 1 个变量时，更偏向使用 `var (...)`，不要用多变量 `:=`；短小且紧贴使用点的临时变量可以酌情例外。
- 代码和实施计划都要有适量的步骤说明式注释，使用陈述句说明当前步骤的动作和目的，尤其是状态机推进、外部副作用、并发/ctx、配置来源和跨系统调用；不要写提问式注释，也不要写逐行复述代码的空注释。

## 分层习惯

- `handler` 只做参数绑定、参数清洗、全部 HTTP 入参校验、调用 `service`、返回统一响应；枚举值、URL 合法性、跨字段必填/互斥等请求参数校验也放在 `handler`，不要下沉到 `service`。参数校验代码量不大时直接平铺在 Handler 函数内，不额外抽 helper。
- `service` 放业务逻辑、数据库访问、内部/外部服务编排；默认接收已经由 `handler` 清洗和校验过的请求结构，不再重复做 HTTP 入参校验。
- `service` 包里不要定义包级常量；业务状态、类型、锁 key、Redis key、超时时间、重试间隔、错误消息模板等常量统一放到 `constUtil/`，可以按业务领域新建文件。只允许在函数内部使用非常短小、一次性的局部 `const`。
- 同一个业务领域优先写在同一个 `service` 文件里，入口函数、回调处理、调度/重试、失败处理、payload 构造等强相关逻辑不要拆到多个 `*Service.go` 里来回跳转；只有跨业务复用的基础设施、独立外部系统适配器或明显独立的子域，才拆成单独 service 文件。
- 函数调用链中已有 `context.Context` 时必须继续向下传递，不要换成 `context.Background()`；启动 goroutine 也要显式传入合适的 ctx，需要脱离请求取消但保留 ctx values 时使用 `context.WithoutCancel(ctx)`。
- `model` 放请求/响应结构和统一业务响应结构。
- `db` 放 Gorm 数据模型、分页结构、数据库初始化，不把 HTTP 入参结构放进 `db`。
- 通用常量和业务错误码统一放在 `constUtil/` 目录里，可以按领域或用途新建文件，不要散落在 `handler`/`service`/`model` 里；`db` model 相关的 enum 常量可以就近放在 `db` 包里，方便看表结构时一起看取值。
- `constUtil/errorCode.go` 维护错误码：`7001-7100` 为系统固定错误码，业务错误码从 `7101` 开始分配。
- 路由通过 `handler` 的 `init()` 注册到 `routes`，例如 `routes.RegisterPostRouter(url, wrapper.Wrapper(fn))`。

## 接口 DTO 与 Swagger 习惯

- HTTP body 入参和出参结构放在 `model` 包；API DTO 的 `json` tag 统一使用小驼峰，例如 `userId`、`callbackUrl`、`itemList`。
- `db` 包里的 Gorm model 字段按数据库列使用下划线 column/tag 没问题，不用为了 HTTP JSON 风格改数据库模型。
- Go 结构体里的 ID、外键 ID、状态值、类型值等整数统一优先使用 `int64`，不要使用 `uint64`，即使数据库字段是 `BIGINT UNSIGNED`。
- 接口必填 body 字段要加 `binding:"required"`，并在 handler 中保留必要的非零校验；例如 `userId`、`requestType`。
- 每个接口都要定义独立的 Request/Response 类型，即使当前字段完全一样，也不要复用一个通用 Request 来耦合不同接口。
- Request/Response 命名按 Handler 名去掉 `Handler` 后缀再追加 `Request` / `Response`，例如 `UserProfileHandler` 对应 `UserProfileRequest` 和 `UserProfileResponse`。
- 接口返回字段要按前端展示需要显式收敛，不要直接返回 db model；内部字段如 slug、uri、时间字段、排序字段等，未明确要求就不要出现在响应里。
- 接口相关的入参、出参字段只要有枚举含义，注释必须写清楚枚举值；出参状态可以写 `0=未知，1=xxx，2=yyy`，入参枚举不要写 `0=未知`，除非 0 是明确可传的业务值；布尔枚举写成 `false=xxx，true=yyy`。
- 每个 HTTP Handler 都要写函数注释和 Swagger 可读取注释，至少包含 `@Summary`、`@Description`、`@Tags`、`@Accept`、`@Produce`、`@Param`、`@Success`、`@Router`。
- Request、Response 以及对外返回 item 的导出类型和导出字段都要写注释，保证 Swagger 文档能读到清晰说明。

## 错误和响应

- Handler 返回 `error`，由 `wrapper.Wrapper` 统一处理错误响应；成功响应在 handler 中直接写 `c.JSON(http.StatusOK, model.NewBizCodeData(data))`。
- 参数错误使用 `model.NewErrParamsError()`。
- 系统错误使用 `model.NewErrSystemError()` 或让底层错误返回给 wrapper 统一处理。
- 业务错误使用 `model.NewErrBizCodeMsg(code, msg)`，错误码定义在 `constUtil/errorCode.go`。
- 成功响应使用 `c.JSON(http.StatusOK, model.NewBizCodeData(data))`。

## HTTP 调用

- 通过 HTTP 访问内部服务时，比如调用 Python 服务，必须使用 `httpClient/http.go` 暴露的方法：
  - `httpClient.HttpInner(serverName)`
  - `httpClient.HttpUserInner(c, serverName)`
- 调外部服务使用 `httpClient.HttpOutput()`。
- 不要在业务代码里直接使用 `utils.NewHttp()` 或绕过 `httpClient` 包；`utils/http.go` 是底层 HTTP client 实现，业务层不直接依赖它。
- 内部服务地址池通过 `httpClient.Init(serverPools)` 初始化，`serverName` 对应 `map[string][]string` 里的 key。

## 日志和配置

- 日志统一使用 `elog`，结构化字段使用 `zap.Field`。
- 配置从 `config.<env>.yaml` 读取，读取失败才回退到 `config.yaml`。
- 新增配置时同步更新 `config/config.go` 的结构体和各环境 yaml。

## 数据库习惯

- Gorm model 放在 `db` 包，一个表一个 Go 文件，结构体字段带 `gorm` 和 `json` tag。
- 表名通过 `TableName()` 显式声明。
- 查询时优先使用 `db.GetIns().WithContext(c)`，保留请求上下文。
- SQL 源文件统一放在 `db/sql/` 目录，一个表一个 `.sql` 文件；新增或调整表结构时，同步维护对应 SQL 文件。
- SQL seed 文件用对应表结构文件的 basename 加 `.seed.sql` 后缀命名，并通过 `.seed.sql` 判断 seed 文件；例如 `<nn>_<table>.sql` 对应 `<nn>_<table>.seed.sql`，不要使用 `<nn>_1_<table>.sql` 或 `<table>_seed.sql`。

## 性能与资源

- 默认沿用 Go + MySQL，不自行增加 Redis、消息队列或独立服务；硬件预算与容量目标从当前 plans 读取。
- 高频请求避免重复检查表结构、加载配置、复制整表和编解码完整存档；同请求复用已有结果，原协议字段与时序保持。
- 只读配置可复用有界、按区服和版本隔离的不可变快照；更新提交后须可见，不能跨库串用或改变已冻结战斗/采集结果。
- 财产、领奖、资源恢复保留必要事务、幂等和锁序；事务内避免密码计算、外部调用及无关重活，鉴权与撤权保持实时校验。
- 列表按需查字段、分页，避免逐条查库；跨页全选保持完整语义，群发等批量任务限制并发并支持幂等恢复。
- 性能改动记录前后查询数、耗时、分配或回包等相关证据；高频/并发路径补代表验证，小改按影响检查，估算人数不作为实测容量。

## 示例代码风格

- `handler/exampleHandler.go` 展示 handler 只做参数校验和分配 service。
- `service/exampleService.go` 展示 service 访问 `db` model 并返回 `model` response。
- `model/exampleModel.go` 展示请求/响应 DTO。
- `db/exampleModel.go` 展示 Gorm 表模型。

## 本项目协议适配（用户明确要求）

- 游戏消息、字段、编号和数值类型沿用 APK 中恢复的定义；生成代码不按通用 DTO 命名规则重命名。
- /sao/ 使用原始二进制 Protobuf 帧和 ErrorMessage；通用 JSON 响应约定仅用于新增管理接口。
- 用户已要求运行数据和运营奖池迁移至 Docker MySQL：正式服务默认 MySQL，存档写入使用 InnoDB 行锁；SQLite 仅保留迁移源与隔离兼容测试（显式 --db）。启动游戏服务无需 Redis 或对象存储。
- Go 固定为官方稳定版 1.27.2。
