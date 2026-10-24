# schema64 只读例外审计

## 结论

主方案可沿用：迁移后建立绑定基础服务 owner 与当前数据库的普通表 positive 快照，普通同库命中跳过元数据，缺表、未知表、未初始化、异库保留动态 HasTable。实施前需主代理决定并处理下列边界。

1. MySQL GetTables 并不限定普通表。`gorm.io/driver/mysql@v1.6.0/migrator.go:418-420` 仅按 TABLE_SCHEMA 查询；原 HasTable 在 `gorm.io/gorm@v1.31.2/migrator/migrator.go:334-342` 要求 TABLE_TYPE=BASE TABLE。直接把 GetTables 所有名称标 positive 会改变同名 VIEW 的行为。建议初始化时一次读取 BASE TABLE 名称；若继续 GetTables 必须另有普通表保证。
2. 原 Gorm RunWithValue (`migrator.go:62-73`) 会继承 tx.Statement.Table/TableExpr，再 ParseWithSpecialTableName。helper若直接用模型 TableName 会绕过显式 Table 覆盖。保守最小做法：显式 TableExpr/覆盖表回动态 fallback；不要自行解析SQL表达式。保留跨库、别名、动态字符串的fallback。
3. 仅检查 snapshot.database==db.GetIns() 不能证明 tx 属于该数据库，调用者可传其他连接的tx，同时全局GetIns未变。需要可靠绑定 tx.Config/root pool 与snapshot，事务内tx.Statement.ConnPool变为sql.Tx不能直接与sql.DB作等值；不可依靠tx==root。ctx owner同样须匹配当前snapshot所有者。
4. positive 快照在运行后 DropTable 会过期。生产源码未发现运行期删表/重命名/临时表；测试确有删表场景。未初始化fixture保持fallback，显式初始化后再删表应标清快照生命周期或显式重建。不要加入negative缓存，否则后续新表不可见。

## 证据与替换范围

- `hyjs-server/main.go:475-491`：最后 InitializeClientUpdates 成功后，当前已有 InitializeGameTableLookup 与 InitializeRedPointSchemaReady。此处可接统一快照；所有迁移已在之前完成。
- `service/adminFieldRuntimeService.go:29-34`：runtime clone通过RuntimeBase继承基础owner，未按值复制atomic；沿用此所有权。
- `service/redPointLatency62Service.go:14-35`：已有快照缓存九模型的true和false，仅比较snapshot.database与GetIns。统一迁移时可让旧helper委托新helper，兼容已有初始化API/测试调用；新positive语义会修正旧negative快照永久缺表问题。不要保留两套互相独立可过期真值。
- `service/globalGameAccountService.go:61-67` 与 `db/gameAccountDatabase.go:11-24`：AccountsTableName可返回跨schema限定名；显式共享schema分支直接true，保持原合同。最小范围可完全保留globalGameAccountsAvailable的现有HasTable fallback，避免将账号库纳入普通游戏schema集合。
- 非字面模型HasTable都可追溯固定模型集合：collectionService:456-466（宝石/料理），adminGuildService:493-502（固定政策），localPlayActivityScopeService:258-282（固定活动来源），p4LuckySupplyService:66-68（固定三政策），adminPlayerInventoryService:828-832（固定三战斗表），p13GuildPrayerService:144-146 与 p13GuildTechService:223-225（固定公会表）。未发现玩家输入生成SQL表名。
- 已有P13 guildCore/Prayer/Tech flags可以保留；只替换其fallback HasTable，避免扩大范围或改迁移/业务数据判断。adminActivitySchemaHasTable同理。
- 生产Go源码扫描无DropTable/CreateTable/RenameTable/TEMPORARY命中；CREATE TABLE和AutoMigrate集中db初始化、Initialize函数及terminal初始化。跨schema账号、钱包、throttle、terminal表名存在配置资格函数，保持它们的边界。
- `service/guildSourceSchema63_test.go:69-86`存在删可选表测试；`service/redPointLatency62_test.go:209-212`有替换ConnPool的计数器，数据库归属判断不能导致计数装饰后无意全部fallback。

最小建议文件：新增schema helper文件；GameService新增atomic快照指针；main在现有startup snapshot附近初始化；service当前普通表HasTable机械替换；redPoint旧helper/API接统一实现。保留db层迁移、所有_test.go原调用、跨库账号特例和既有ready短路，不改业务查询/写入、表结构、JOIN或迁移顺序。单条SQL JOIN最多3个（≤3）保持。

## 必要验证（本代理未执行）

- 同owner同库启动后model/string/tx事务及runtime clone命中零元数据SQL。
- 未初始化、异owner、另一数据库tx、全局数据库切换、显式tx.Table/别名/跨schema、未知模型均fallback且返回真实结果。
- 缺表初查false后运行时创建，下一次应动态true；快照不保存negative。
- MySQL同名view保持原HasTable=false；GetTables错误时不发布不完整快照，继续fallback。
- 复用已有redPointLatency62/guildSourceSchema63代表检查，并核上述ConnPool装饰不让快路径丢失。
- 原消息端到端计数/耗时由主代理统一候选验证；只读审计不能宣称点击延迟已解决。

## 验证未做 / 剩余 / 资源

仅执行rtk前缀只读文件和依赖源码检查；未运行编译、测试、DB/Redis/Docker、浏览器、手机、部署；未改产品源码。已写本报告。剩余为主代理确认view/tx覆盖与数据库归属实现后明确分配实施。没有创建进程、数据库、容器、账号或缓存，无资源待清理。

## Root批准后的实施（2026-10-24）

- 已实施普通表正集合快路径。MySQL一条information_schema查询，限定TABLE_SCHEMA=DATABASE()且TABLE_TYPE=BASE TABLE；SQLite一条sqlite_master type=table查询；其他dialect不发布缓存。初始化错误清空snapshot并返回main。
- 快照绑定base owner、db.GetIns指针、Config.ConnPool。仅已知*sql.DB支持快路径，普通Begin得到*sql.Tx且保留同Config.ConnPool可命中。未知ConnPool装饰、异owner/库/全局库切换、显式Statement.Table或TableExpr、限定/复杂字符串均走原HasTable；缺表不保存negative，新增表下次可见。
- 225处普通service HasTable精确替换，91个原业务文件只改包装调用。第一次脚本224处，剩余p13GuildService.go WithContext链式receiver逐处补1处；globalGameAccountsAvailable跨库特例原样保留，helper原始fallback保留。没有修改业务SQL、写入、权限、数据资格或现有ready短路。
- main在原startup redPoint初始化位置一次调用InitializeGameSchema64并返回error；runtime clone继续继承RuntimeBase。旧InitializeRedPointSchemaReady测试API委托新初始化，失败保持fallback；旧redPointHasTable绑定接收者owner后调用统一helper。Root批准清理无引用旧redPoint快照结构/atomic字段/import。
- 精确文件与替换数清单：`analysis/latency64-20261024/schema-files64.txt`。新增gameSchema64Service.go与gameSchema64_test.go，另改gameService.go、redPointLatency62Service.go及main.go接入。
- 静态证明：逐91文件读取HEAD，进行同一精确receiver替换并gofmt，与当前源码比较没有额外业务差异（额外WithContext链式补丁已单独标记）；git diff --check通过。全service生产源码剩余原HasTable只有global账号兼容与helper底层fallback。
- 已通过一次真实SQLite隔离测试TestGameSchema64OwnerDatabaseAndDynamicFallback，覆盖未初始化、model/string命中、clone、另一owner/无owner、Table覆盖/alias、缺表及运行新增、view、普通事务、另一数据库/事务、Config pool变化与未知装饰、全局库切换、关闭连接初始化失败且snapshot nil。首次测试计数fixture在安装logger前已创建tx导致未捕获元数据，修正logger安装顺序后通过，产品逻辑无需为此变更。
- 最后死结构清理后的同一schema单测再次通过（1 passed）；Root负责MySQL与协议端到端/基线benchmark、合批全包验证。此前只读审计的“未实施”状态至此由本节明确取代。
- 没有新增服务/容器/账号；SQLite文件由t.TempDir回收，连接显式关闭。没有部署。Root的新后台签到与怪物/Boss业务由其他writer接续，本代理释放原service文件机械替换所有权后不再写业务文件。
