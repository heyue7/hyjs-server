# 沙漠搜打撤 V4 实施

当前状态：按用户断网前要求，已停止开发与远程操作，待用户继续。

用户于2026-10-28授权按V4实施，交付正式APK、上线服务端、现有后台沙漠参数全可调，后台按ui-polish精修。允许UU远程Unity。

## 固定起点

独立工作树desert-extraction-v4，已发布基线07eb98e；根目录混合差量保留。已批准方案：根plans/deliverables/desert-extraction-20261028/index.html和design-data.json。

## 所有权与验收

- desert_server_v4：hyjs-server及必要定向测试；schema3草稿迁移、真实运行消费、公共资源/战斗/芯片/交易/资格/结算/性能。
- desert_client_v4：tools/desert_extraction和沙漠桥、analysis/desert-extraction-v4-client；原SAO UI与原加载动画、地图桥重绑及运行链。
- desert_unity_v4：UU远程Unity独占，核导航、采集岸线、Boss与桥怪模型；不改原地形与导航。
- 主代理：admin-web沙漠参数编辑、信用点int64与商店冻结涨价、集成、真实MySQL/原机及发布。代理不可独立部署，重型检查/手机串行。

## 后台设计记录

旧schema2参数分散且待定文字过多。新版将资源数量/双刷新/倍率与阶梯放一起；小怪分档折叠；唯一Boss属性/日程与奖励放一起；六色品质/兑换可逐行比较。沿原Naive UI，保留草稿保存与区服发布语义。以桌面/320窄屏真实页面、保存/重开/发布回读验证。未渲染前不判UI完成。

## 当前事实

UU已进入远端Unity并实际执行多轮坐标/模型/导航检查，五项工程基线一致；此前提示不能作为设备离线结论。59点首轮47通过，后续同水体鱼点与Boss同区候选已补，最终联合点位及原机仍待验。原模型Editor着色器不支持完整渲染，诊断材质截图只验证几何。没有发布沙漠、未给出可用APK，也未完成原机验收。

## 本机资源回收（用户反馈卡顿后）

- 本批Go编译/测试、18089/19099隔离服务、19589前端预览、验收浏览器守护进程23276均已结束，端口已核释放。UU主程序已退出，系统共享服务保留。
- 独占Go缓存7696文件、旧服务二进制、日志交叉测试二进制、Vite缓存已清，逻辑回收2.93GiB；客户端另清旧重绑r1和失败预检约5MiB。保留最新测试基线、当前overlay、源码、私有配置、隔离库、业务与审计证据。
- 后续重型编译/数据库回归串行，GOMAXPROCS=2、GOFLAGS=-p=2、GOMEMLIMIT=2GiB；不同时运行多组全service编译。需要图像/浏览器验收时才开启，结束立即关闭。
- 当前仍未生成可交付V4 APK、未上线沙漠；大余额随机抽样优化、公共怪/Boss/交易/撤离及原机全链仍在实施。后台草稿保存回读、320px重排已验；信用迁移/报价基础定向检查已过，后续变更待重验。

## 18:40 后续验证与释放

- 用户明确允许正常使用资源，要求及时释放，开发继续。远端UU当前由desert_unity_v4按需占用。
- 信用点迁移/回滚、大于int32、8×4并发加减及冷重连；商店购买幂等/并发与冻结报价恢复，四项真实MySQL回归全部通过。固定测试二进制复用，各项隔离库与账号逐项释放，75MiB测试二进制已删除。细节与源码指纹：analysis/desert-extraction-v4-finance/verification.json。
- 后台按独立截图审阅补窄屏固定保存条，并明确公共节点次数。320×900实际90→91保存回读，再恢复90保存回读；明暗主题无整页横向溢出；最新typecheck通过。证据：analysis/desert-extraction-v4-admin/polish-verification.md。
- 本次后台复核使用旧a18固定二进制，未覆盖新加入掉落字段与公共怪服务。浏览器/前端/测试服务全关闭，18089/19099/19589端口已释放，旧63MiB验收二进制及其Vite缓存也已清除。a18库保留给最终集成，仍由本轮持有。
- 公共怪真实战斗、断线续局、Boss校准/共享结算、交易/撤离、地图最终点位、最终后台全字段、APK与线上验证仍在实施，不扩张上述局部通过范围。

## 用户断网前收尾（2026-10-28）

用户要求尽快结束，已停止新增实现、构建和发布。所有源码保留在本工作树，根目录未集成。`GGOExtractionReady=false`。当前没有可交付的V4 APK，没有上线V4。

- 最后一组合批 `go test ./service ./handler -run '^(TestGGO|TestSharedFieldBoss55|TestFieldBossDeath73)' -count=1 -timeout=4m` 已通过：service5.300s、handler0.811s。仅匹配到的隔离测试与编译，不代表真实原机闭环。
- 主代理新增Boss事件/资源/芯片/原type12开战及结束/active与receipt，复用旧表扩列，未经过真实Boss端到端。新的Boss资源/轮数/小芽/超频参数已接后台，最终渲染尚未复核。
- 客户端静态预览r3b保留547原引擎方法；23处限定入口钩子尚未打包和原机验收，Boss投影与桥巡逻未完成。断网保留活引擎+原结束包持久重试采用最小方案；进程被杀不承诺帧级恢复。
- Unity坐标候选与报告在analysis/desert-extraction-v4-unity；F11越界重选r5待核，西部鱼点/桥遮挡及玩家视角可见性仍需确认，所有新增点仍未批准。
- 交易刚接入，双方并发/断网/原子回执端到端未验；采集、PvP、撤离并发、实时公共节点推送、Boss50万战力15人校准、≤1秒原机数据仍未完成。
- 已确认版本门禁缺口：旧APK新session不能凭持久GGOField进入新玩法。续接时在最终APK版本确定后接build+session绑定和强更，当前未猜测版本号。
- a18隔离数据库和临时账号现已删除；未来重新申请自己的测试库，不能复用旧私密连接。财务四项的临时库账号也已全部释放。临时后台浏览器、服务与预览此前已全部关闭。

续接先核本记录和三个执行代理最终handoff；主代理负责先审查新Boss结算、真实MySQL/原协议及后台全字段，然后完成APK/原机/上线，不能把本批局部通过当已交付。

### 最终本机状态

- 最新后台 `pnpm typecheck` 通过；本机 Go/Node 检查均结束，无本轮测试服务或浏览器。18089/19099/19589已核释放，a18库/临时账号/私密bootstrap已清理。
- 客户端最终交接 analysis/desert-extraction-v4-client/STATUS.md：FightEndJournal.cs 在r3b预览后有一处保存失败恢复修改，当前源码不等于该预览清单，必须重构核验。
- Unity最终交接 analysis/desert-extraction-v4-unity/HANDOFF-20261028.md：UU前台已退出；r5仅本地候选，未远端复审。远端本批临时脚本与候选输入保留供续接，避免断网前误删当前引用资源。
- 未接入交易集成测试已移到 analysis/desert-extraction-v4-server/trade-integration-test-draft.go.txt。
- Boss新消费代码所在 service/ggoBossService.go、ggoBossBattleService.go、ggoBossRewards.go；HTTP /ggo-field/boss/active与/receipt。最后Go组合已编译这些源码并跑匹配隔离测试，仍缺MySQL并发末击/原type12真实包、实际领奖、原机可视交互和50万战力校准。
