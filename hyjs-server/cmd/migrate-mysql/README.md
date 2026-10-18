# SQLite → MySQL

游戏服默认使用 MySQL。通过环境变量提供 `HYJS_MYSQL_HOST`、`HYJS_MYSQL_USER`、`HYJS_MYSQL_DATABASE`、`HYJS_MYSQL_PASSWORD`，密码不接受命令行参数。`--db <临时文件>` 继续使用 SQLite。

停服并备份源 SQLite 后，在 `hyjs-server` 目录执行：

```sh
rtk proxy go run ./cmd/migrate-mysql --source /absolute/source.sqlite3 --apply
```

迁移工具读取源库一致性快照，原样保留账号 ID、大小写及尾空格、会话、密码 salt/hash、JSON 文本、时间戳和 NULL/空 blob 的区别。缺少早期可选列时使用 NULL。支持 profiles 和可选的 gameplay_pools/gameplay_pool_rewards；未知表直接拒绝，避免遗漏数据。

相同 ID 或 UID 已存在时，所有字段完全相同才允许跳过；任何差异都会回滚整批数据，不提供强制覆盖。自增序号取源 sqlite_sequence、源最大 ID 和目标现有序号的最大值。MySQL 的 AUTO_INCREMENT DDL 在数据事务提交后执行；若这一阶段失败，保持停服并重跑即可恢复。目标表必须使用 InnoDB、唯一二进制 UID 和完整的大字段类型。

去掉 `--apply` 可检查已有目标库并输出待插入数量。该模式要求目标 profiles 表已经存在。报告只显示记录数、历史序号和 SHA256 摘要，不输出会话或密码。应用启动后会修改账号会话或存档，因此应在启动前执行最后一次校验。

隔离验证：创建并授权专属 `hyjs_test_*` 数据库，设置 `HYJS_MYSQL_TEST_DATABASE` 后运行 `rtk proxy go test ./db ./service -run TestMySQL -v`。额外设置 `HYJS_GO_TEST_BINARY` 指向独立临时服务 binary，可运行 `tests/test_mysql_gameplay.py`，验证原 Python 战斗存档、招募、转盘及并发回执。测试完成后删除专属库并撤销对应授权。
