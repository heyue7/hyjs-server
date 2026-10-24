# Re 1.1.11 豪华签到只读核查

## 可直接回答

找到了：这版 Re 客户端的豪华签到展示和本地规划奖励都是 **1500钻石（diamond,1500）**，固定数量，相关分支无等级读取、等级倍率或随机奖。条件为 **当天真实消费500钻石**，每个UTC+8自然日可领取一次。它与本服当前6元支付门槛使用不同资格来源。

需保留边界：最新版普通领取请求会先发给Re远端活动服务；远端成功响应的 `activity.reward` 覆盖客户端规划结果并提交。APK能证明客户端展示/预期1500钻，不能单独证明当前远端服务实际仍发这个数值；本次未请求其服务器、未实际领取。不能把该1500自动作为本服原运营数据配置。

## 来源与静态证据

- 唯一APK：`/Users/lirui/LR/hyjs/apks/original/黑衣剑士-Re-1.1.11-精灵商店修复-20261022.apk.1`，SHA-256 `2d32f6babaae52113ffe144fa79fa139c5244ebb4b80bc1d07376c2e7a23b257`，1,686,276,994 bytes。
- 只提取成员 `assets/bin/Data/Managed/OfflineBootstrap.dll`，SHA-256 `b26c6b453e5a534bcdfa7da5d302f9266c9ade88e813129d6ecd321ebe59179b`，19,486,208 bytes；MVID `4a8ad10f-be87-4380-8817-174c42135812`。身份详见source-identity.json。
- REA artifact identity父Evidence：`ev_8e482dd7a85b2fc66555e5f0b325eedff63838dafb18f8d784f48fba6be05213`；focused OfflineEvents315/OfflineGrowth父Evidence：`ev_782be2c51c6ae3e41ccd61f78031f6f27899698edef4db9dcd03dda7c048b82d`，195 methods，metadata complete；#US一条terminal-flag warning，保留为解析限制，不影响本次相关可读UTF16常量。nested display types见rea-nested-summary.json（命中7个同名元数据type，按本报告token绑定目标）。
- dnfile/dncil独立解析同一成员，相关511个方法均可解析，未执行目标程序集。逐指令证据见luxury-focused.il.txt，较广原始IL见protocol_methods.il.txt；token/RVA均只在上列SHA与MVID内有效。

## 展示、数量与路径优先级

1. `OfflineGrowth.Request` token `0x060006e3` IL `000c–0024`先要求服务器时间；`00a5–00b6`优先交 `OfflineEvents315.Request`。
2. 豪华列表请求57001属于EventCodes。`OfflineEvents315.Request` token `0x06000217` IL `0267–0275`调用 `OfflineGrowth.Dispatch315`时传入包装callback `<Request>b__0`。
3. `Dispatch315` token `0x060006e4` IL `03e5–043f`确有占位 `luxuryNeedPay="0" / luxuryReward="coin,1" / luxuryState=2`；这不是正常UI最终收到的值。包装callback `0x06000d86` / RVA `0x1cd708` IL `0027–0096`在转交UI前覆盖为 `diamond,1500`，剩余条件 `max(0,500-Count("spent.day."+Day))`，已领状态2、达标1、未达0，最后 `009b–00ab`调用原callback。
4. `LuxuryClick` token `0x0600022a` IL `0019–0058`不足500时展示消费500钻/返1500钻文案并跳商店；达标后 `0059–006f`发送57015 / CSReceiveLuxurySign。
5. `OfflineEvents315.Request` IL `03be–0423`再次校验当天消费>=500，`03e7–03ec`固定Reward("diamond,1500")，构造SCReceiveLuxurySign.get。此处无等级数据读取。Reward token `0x06000214`将资源ID和固定int数量解析成ResultObject，未乘角色等级。

## 每日幂等与远端权威

- 领取action `<Request>b__163_3` token `0x06000d71` / RVA `0x1ccfe4` 调 `Claim(Daily("luxury"))`；Daily token `0x060001bd`形成 `d:<yyyyMMdd>:luxury`。Day token `0x060001b0`读取Local并格式化yyyyMMdd（Local使用服务器时间+8小时）。Claim token `0x060001bc`在Claims已包含key时抛“此档活动奖励已领取”，否则加入；执行事务带回滚。
- `.cctor` token `0x0600023e`通过FieldRVA初始化ServerMutations306；event-code-arrays.json从PE原始数据解出21个int，明确包含57015（普通领取），也解出含57001/57015的EventCodes。未凭字符串猜路由。
- `Request` IL `0040–005c`在非planning且Mutating306时直接 `ServerRequest306`并返回；该函数token `0x060001a1`调用OfflineCloud.Activity306。
- planner `<ServerRequest306>b__0` token `0x06000d82`设置planning306=true，再进入本地Request，生成上述1500预期对象，finally清planning并Restore之前Record；因此规划过程不会自行持久发奖。
- apply `<ServerRequest306>b__1` token `0x06000d83` IL `003e–0053`从远端 `activity.reward`解析ResultObject，`0102–0125`恢复远端event、Transaction.Commit并ApplyServerTransaction310，`013a–0175`通过反射覆盖规划reply的get字段/属性。实际奖励以这一响应为准。
- 成功UI callback `<LuxuryClick>b__0` token `0x06000da8`读取SCReceiveLuxurySign.get展示；失败不进入成功展示。静态正常路径存在，依赖时间同步、消费门槛、未领取、联网及远端确认；未运行不能宣称已实际到账。

## 与本服6元门槛的区别

本服当前源码 `paymentOfferService.go:214–248` 使用PaymentTotals累计当天已确认支付cents，并读取GameConfigure/LuxurySignRechargeAmount换算人民币门槛；monthSignService.go:209当前回包NeedPay="6"。本服奖励来自ActivityRules/ActivityReward配置（paymentOfferService.go:185–211），按luxury-sign+p3Day幂等。本次未查DB、未修改本服门槛或奖励。

## 未做与资源

仅静态读APK/程序集及本服相关源码；未执行程序集、联网领取、启动DB/服务、编译、手机/浏览器操作或修改产品数据。保留约19.5MB helper小副本和本次必要证据；没有新增进程、容器、账号或缓存。旧RE1051仅用作定位提示，本报告每个结论均回到最新SHA重新核对。
