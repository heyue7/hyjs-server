# 66期：试炼结算、活动统计和商店购买

状态：2026-10-25已交付。代码 `92995a79ff480454363939f1b57cd8e7d0664e59` 已部署，运行Go SHA256 `66e183760c844eb8472e49cb71cf60621dcbc50d405a23085b661304c989d51c` 已独立核对。本记录的后续提交只补交付证据，无需重新部署。

正式APK：1.0.66 / build2026102501，`build/hyjs-feedback66-formal-r7.apk`；SHA256 `60be20022b95530882b9197d1e9a37c5c8659c061f71e5386938e117262885b4`。用户入口 `/Users/lirui/LR/hyjs/build/hyjs-local.apk` 与固定交付文件为硬链接，已在Vivo V2507A安装并核验实际安装摘要、启动页面。

基线：db2db45 / 正式1.0.65(2026102405)，已合入并保留线上dd6de17月签到日历配置及RE默认奖励。目标为恢复试炼结算与继续入口、纠正活动进度/消费事件、商店购买和限购展示，以及活动副本体力显示。

## 实际新增行为

- 试炼自然复现：原客户端后续玩家回合也可能等待30秒再开启自动；旧Playback每tick只消费一条命令，旧LiveAuto只支持首轮等待，导致真实战斗被拒绝。新增逐玩家回合LiveAuto独立驱动，仅以各回合首成功时刻作为唯一控制候选，原AI独立构造队列、重算目标、完整录像和终末HP。没有时间搜索、舍入或相等条件放宽；原模型、JSON、hash、proof及已有分支优先级保持，新回执使用独立driver/proof及来源digest。
- 普通与精英试炼冻结原Start/End字节，并按账号、实例及回调代际隔离。明确拒绝后由原挑战按钮继续同一付费实例；提交结果未知时重试原End，避免新Start。主挑战按钮提前拦截恢复，覆盖服务器已结算且楼层已推进的情况。Start超时释放等待，同代迟到回复仍可正常完成。
- 开服第六天type24按角色已确认的历史最高星数更新，试炼End事务内同步；重置保留纪录，活动宽限保持冻结。原任务40/45/50统计的是星数。
- 积分商店canBuy恢复原客户端枚举1，恢复原购买按钮及数量弹窗。声望和积分商品按原字体展示“今日剩余/总限购”，保留已购账本。后台dailyLimit可配置；声望碎片线上继续20份/日。RE远端鉴权商品目录未取得，不能将20宣称为RE规则；37仅用于隔离QA验证。
- 基金、开服抢购、世界首领150钻加成补全真实消费事件，与购买事务幂等提交，沿用UTC+8消费窗口。用户所述旧7000消费缺可信发生日期证据，未伪造历史补录。
- 悬赏活动按最终击杀计数。核查角色24为4次攻击、1次最终击杀，显示1符合原规则，未改成攻击次数。
- 活动资源副本Start已扣10体力，原客户端结果页又按End.Cost本地Delete，造成显示少20。仅在网络回复投影去除已预扣的stamina，冻结回执仍保留真实Cost10供严格重放；奖励和校验保持。

## 实现与验收

| 单元 | 负责人 / 文件范围 | 实现 | 验证 | 发布 |
| --- | --- | --- | --- | --- |
| 逐回合自动驱动 | Root；constUtil/p7RoundLiveAuto.go、service/p6RoundLiveAutoService.go、service/p7RoundLiveAutoRushService.go及少量原分支入口 | 完成 | 原机真实录像、独立原引擎、Go、MySQL及冷重试 | 92995a7已部署 |
| 试炼原按钮恢复 | Root；tools/client_feedback66/TrialRuntime.cs、TrialInjector.cs、build.py | 完成 | 明确拒绝恢复、真实旧失败帧恢复、已提交丢回包恢复 | formal-r7已装机 |
| 活动和商店 | Root集成；activity66_diag、commerce66、trial66_diag只读调查 | 完成 | 真实协议/MySQL、原控件实购、历史规则核查 | 92995a7及formal-r7 |
| 活动副本体力 | Root；预扣体力回复投影 | 完成 | 胜败、重复、冷恢复；原机100→90 | 92995a7已部署 |

OpenCode Free运行run-tw1lplb2明确quota429退出且无实现，按项目AGENTS授权使用gpt-6.1-sol接续调查。Root持有代码集成、服务端、手机和发布资源；代理均已结束。

- 合批真实协议/MySQL检查通过（go-integrated-r7.json）；合入dd6后月签到及活动/商店增量通过。活动副本胜败、同场Start恢复、重复End、冷恢复通过（go-dailycopy-final.json）。最终新增驱动与旧Generic/manual回归通过16.905s；Go构建、vet及部署Linux构建、前端检查通过（go-final-source-checks.json）。
- 两份第7层原机实际记录在独立原547宿主与Go比较：全部29 press、28 skill_begin目标、四演员HP/state及完整协议相等（round-live-auto-native-r2/go-native-strict-comparison.json）。原547引擎、26934非目标raw方法一致，最终13种新增Hook签名作用域检查通过（qualified-managed-overlay.json）。未发布候选曾发现的HttpHandler签名JIT错误已修正；正式65无此新增错误。
- 原机明确拒绝后恢复同一第7层实例，7→8层、6→7星，5500金币一次；真实第8层旧失败End在升级后原按钮重试成功，进入第9层8星，6000金币一次。第8层同End重复两次及冷服务器重启后回包摘要相同，钱包、星数和实例数不变（mysql-new-driver-repeat.json）。隔离HTTP热786/778ms、冷1142ms，不能泛称全部接口低于1秒。
- 原机第9层真实End在服务器提交后由隔离代理丢弃回包；界面已第10层9星，原主挑战按钮提交完全相同End成功恢复，未新开Ready/Start、未重复发奖。正常选择原生命+3%必选加成，未绕过玩法（phone-committed-drop-retry-passed.json）。
- 原机声望QA商品16→15/37，积分17→16/20，原批量弹窗和实购通过（shops-qa.json）。
- 原机经验水晶副本真实胜利、原奖励和迟到结果回调后，体力100→90，数据库90，单场费用10且仅结算一次（phone-daily-stamina-passed.json；phone-daily-returned-90.png）。
- 正式包签名、对齐、5DEX检查通过，实际安装SHA一致（formal-phone-installed.json）。新异常恢复原机锚点为普通试炼；Elite共用恢复代码及typed10诊断投影已检查，本轮未覆盖Elite完整原机战斗及任意手动控制组合。模拟器/平板全机型兼容未在本批扩称通过。

## 发布与存档核对

部署记录deployed.json，前后快照live-before.json、live-after.json。健康、后台和静态资源200，CORS204；运行二进制SHA与构建清单一致，应用OOMfalse/restart0。活动启用18/归档35保持，P7 pending2、Wanted pending2/committed4保持。角色1存档摘要保持；角色24玩家标量摘要因时间/在线重连变化、游戏存档摘要保持，不宣称全部标量字节不变。无生产游戏登录、公告、邮件、发奖或补偿。

## 资源归还与磁盘清理

- 手机原8偏好、caller、反向端口和显示设置已恢复，游戏停止并回到桌面（phone-restored.json）。隔离QA服务/失败代理停止，临时MySQL库和账号已释放；共享MySQL/Redis保留（qa-cleanup.json）。
- 归属本项目、超过6小时且无活跃编译或打开引用的旧Go归档缓存清理53,324,231,658字节。仅删除检查过HYJS模块标记的归档，近期输出和其他项目缓存保留。
- 废弃候选APK、QA包、打包overlay、中间件和恢复后已无用途的手机备份共清理6,958,454,918字节；本机合计60,282,686,576字节（约60.3GB），当时可用55,399,825,408字节。固定64/65构建输入、当前66交付、源码、凭据与必要证据保留；APK别名使用硬链接。
- 服务器两份旧加密备份共清理956,168,771字节。检查主机打开文件及运行/停止容器挂载引用后，仅保留本次部署前一份已验证加解密的备份pre-deploy-92995a79ff48-1792871339450011148.sql.gz.aesgcm，SHA256 c1a04406e19e00b5dc8ed8576e7d509e9ca0928b55bfff36a373465200d3ebec。未执行全局prune/flush。
- 汇总：delivery-cleanup-summary.json、local-final-cleanup.json、remote-cleanup-final.json。当前没有本批占用中的测试资源。
