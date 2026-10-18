# 灵魂之花 Go 兼容服务器

入口为 `main.go`，使用 Gin、Gorm 与原 HTTP/TCP Protobuf 协议。
完整安装与部署步骤见 [项目 README](../README.md)。

```bash
cp config.example.yaml config.dev.yaml
# 编辑私有 YAML 或导出 HYJS_* 环境变量
go run . --env dev
```

配置优先级：显式命令行参数 > 环境变量 > YAML。`HYJS_CONFIG_FILE` 可指定私有文件；未指定时依次读取 `config.<env>.yaml`、`config.yaml`。实际 YAML 已被 Git 忽略，仅提交 `config.example.yaml`。

主服务使用 MySQL 8.4。`--db` 显式启用隔离 SQLite 兼容路径；Redis 与存储适配器为可选依赖。`analysis/tables/` 提供游戏配置，`server/protocol/` 保存原消息定义。

`handler/` 处理协议与参数，`service/` 实现业务和事务，`db/` 保存模型与迁移，`model/` 保存请求/响应，`constUtil/` 保存类型、常量及错误码。原协议字段与编号保持客户端定义。

后台通过 `HYJS_ADMIN_ASSETS` 指定静态目录，通过 `HYJS_ADMIN_BOOTSTRAP_FILE` 指定权限为 `0600` 的首次管理员文件。仓库不含实际管理员账号密码。
