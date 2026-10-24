# 后端研发约定

继承根目录 `AGENTS.md` 与 [hyjs-development](../skills/hyjs-development/SKILL.md)。本文件保留稳定规则；版本、测量结果和本批进度放在 `plans/`，实现示例与性能检查见[数据库与性能指南](docs/development/database-performance.md)。

## 1. 技术与交付

- Gin + Gorm，入口 `main.go`；Go 固定为官方稳定版 1.27.2。正式服务使用 Docker MySQL / InnoDB 行锁；SQLite 仅作迁移源和显式 `--db` 的隔离兼容测试。游戏启动无需 Redis 或对象存储，默认不增加缓存服务、队列或独立进程。
- 修改后先 `gofmt`，开发中检查受影响路径；依赖差量合批完成全包编译、`go vet` 和构建。共享核心广泛变化、影响不明、关键里程碑及最终交付执行全量测试；已有可信同源基线可复用并补验增量，如实说明范围。
- 依赖整理使用 `go mod tidy`，不手改 `go.sum`。Shell 使用 `rtk` 前缀；本机数据库与资源清理继承根目录规则。
- 多变量声明优先 `var (...)`，紧贴使用点的短临时变量可例外。注释说明步骤动作与目的，重点交代状态推进、副作用、并发/ctx及配置来源，避免逐行复述和提问式注释。

## 2. 分层与上下文

- `handler` 负责绑定、清洗和全部HTTP入参校验（含枚举、URL、必填/互斥），调用 `service` 并统一返回；小段校验直接平铺。`service` 接收已校验DTO，负责业务、数据库和内部/外部编排。
- `model` 放独立接口DTO及业务响应；`db` 放Gorm表模型、分页及初始化，禁止混入HTTP入参。每张活动表一个模型文件，`TableName()`显式声明，带 `gorm`/`json` tag。
- 同一领域的入口、回调、调度、失败处理与payload优先放同一service文件；独立子域、跨域基础设施和外部适配器再拆分。
- 包级业务常量、状态、锁/Redis key、超时、重试和错误模板统一放 `constUtil/`；service只允许极短的一次性函数内局部常量。表模型枚举可就近放 `db`。
- 调用链已有ctx就继续传递；goroutine显式接收合适ctx。脱离请求取消但保留values时使用 `context.WithoutCancel`，不换成 `Background`。查询沿用 `db.GetIns().WithContext(ctx)`。
- 路由在handler的 `init()` 通过 `routes.RegisterPostRouter(..., wrapper.Wrapper(fn))` 等入口注册。

## 3. 协议、DTO与文档

- APK恢复的游戏消息、字段、编号和数值类型保持原定义，生成代码不套通用DTO重命名。`/sao/` 保持二进制Protobuf帧与ErrorMessage；以下JSON约定用于新增管理接口。
- 每个HTTP接口独立Request/Response，命名为Handler去掉Handler后缀再加Request/Response；HTTP body结构放model，JSON用小驼峰。数据库列继续用下划线。
- 自定义结构的ID、外键、状态和类型整数优先 `int64`，避免 `uint64`；原游戏生成类型保持。必填字段用 `binding:"required"` 并在handler保留必要非零校验。
- 返回字段按展示需要收敛，不直接暴露db model；内部slug/uri/时间/排序等字段未明确要求不加入。枚举注释写清取值；布尔写false/true含义，入参不增加无实际用途的“0=未知”。
- Handler及导出DTO/item/字段写可供Swagger读取的注释，Handler至少包含Summary、Description、Tags、Accept、Produce、Param、Success、Router。

## 4. 错误、HTTP、日志和配置

- Handler返回error，由wrapper统一处理；成功 `c.JSON(http.StatusOK, model.NewBizCodeData(data))`。参数错误用 `model.NewErrParamsError()`，系统错误用 `NewErrSystemError()` 或透传，业务错误用 `NewErrBizCodeMsg(code,msg)`。
- 错误码集中 `constUtil/errorCode.go`：7001–7100系统固定，业务从7101开始。
- 内部HTTP使用 `httpClient.HttpInner(serverName)` / `HttpUserInner(c,serverName)`，外部使用 `HttpOutput()`；地址池经 `httpClient.Init(serverPools)` 初始化。不直接调用底层 `utils.NewHttp` 或绕过httpClient。
- 日志统一elog + zap.Field。正式环境关闭SQL逐条回显；移动/心跳等高频成功请求不逐条info，保留错误、慢请求与必要审计，并配置轮转/容量上限。
- 配置先读 `config.<env>.yaml`，失败才回退 `config.yaml`；新增配置同步 `config/config.go` 和各环境yaml。

## 5. 表设计和迁移

- 先核基数、归属、生命周期、实际查询/写入、索引和锁序。固定一对一、同主体且总是一起读取的少量标量优先合并到主体表，避免为单个开关拆表；冷热/历史/一对多/账本按实际边界分开。禁止类型列驱动的多态巨表及把钱包、流水、战斗历史塞进角色存档。
- 表数量与事务数量分别评估。已有同一事务可直接多表原子提交；合并表的收益须以少查/少写/更短事务证明，不以表数推断。保留财产、领奖、资源恢复及结算的必要事务、幂等与行锁。
- 合并前列出唯一约束、外键引用、筛选/排序索引、并发修改和回退路径；小字段合并使用列级更新，避免Save旧主体快照覆盖其他字段。批量读取保留索引，避免逐条关联查询。
- 结构SQL放 `db/sql/`，每表一个结构文件，与模型同步；seed用同basename加 `.seed.sql`，不采用 `_seed.sql` 或额外序号命名。
- 存量迁移须有可验证备份、可重复/中断恢复的回填、来源与目标语义核对、兼容读写/回退。禁止仅凭“列存在”认定回填完成；旧表确认无读写、引用及回退依赖后再移除。
- 数据清理限定项目库与明确表、过期及引用条件，先预览再分批短事务执行；业务资产、历史、幂等回执和必要备份不当缓存删除。

## 6. 请求性能与资源

- 单条SQL最多3个JOIN（JOIN数量≤3，超过3个禁止）；复杂关联按数据归属调整表结构或改为有索引的批量读取，禁止拆成逐行N+1查询。此限制同时覆盖Gorm链式关联、Raw SQL、子查询和公共查询构造器，审查最终生成的SQL。
- 延迟优化覆盖全部常用交互和公共请求链。SQL慢时检查索引、读写列、关联设计、锁等待与重复事务；表设计有问题就调整表及迁移，接口编排或计算有问题就调整接口和业务逻辑。按同输入的端到端耗时验证，不能只修改加载提示或把慢工作转移到后续请求。
- 同请求复用鉴权、profile、配置与事务；避免移动前再做一遍资源事务、为心跳建立活动事实、无效活动先查资格。先筛种类/启用/冻结时间窗口，再处理相关业务。
- 无变化不UPDATE；比较实际持久化内容并只写变化列，保持原资源恢复基线、冻结事实和账本钩子。不要靠跳过校验或异步丢写提速。
- 高频路径不重复检查表结构/加载配置/复制整表/编解码完整存档。配置缓存有界、区服/库/版本隔离，提交后失效可见，冻结战斗/采集继续使用原快照。
- 大段冻结输入采用有界无损压缩及原文摘要，按需解压；结算后仅清除可重建冗余，保留结果与幂等依据。摘要、权威判定、奖励与冷恢复语义保持。
- 事务内避免密码计算、外部调用及无关重活；多表操作传入已有tx，避免嵌套/重复事务。鉴权与撤权实时校验，外部通知在提交成功后发送。
- 列表按需查字段并分页，关联信息批量读取；跨页全选保持完整语义，群发等任务限制并发、支持幂等恢复。
- MySQL连接、buffer pool、表缓存、临时表RAM与容器上限一并预算，不以提高内存上限掩盖无界查询。保留持久化/恢复合同，不擅改共享实例。
- 优化记录相同输入的SQL/事务数、耗时、存储字节及必要内存证据，覆盖并发/失败/冷恢复；局部测试与冷启动快照不扩大为整服延迟或容量结论。结束及时停自有进程、释放测试库与临时资源。

参考示例：`handler/exampleHandler.go`、`service/exampleService.go`、`model/exampleModel.go`、`db/exampleModel.go`。
