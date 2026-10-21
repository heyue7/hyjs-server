# 蓝绿终端 Android 浮窗

独立原生 Java 层，`WalletActivity extends com.unity3d.player.UnityPlayerActivity`。
只给当前 Activity 增加48dp圆形半透明入口，吸边后只露半球；触摸/打开时全亮，关闭1.5秒后回到40%不透明度。钱包是纯白、蓝色主操作的独立页面，无WebView、无系统悬浮窗权限。
原Unity场景及Battle.Engine不在本模块修改范围。

## 认证与 API

C#桥在真实登录/重连成功后调用：

```csharp
using (AndroidJavaClass wallet = new AndroidJavaClass("com.hyjs.wallet.WalletActivity"))
    wallet.CallStatic("bindSession", StaticData.playerData.playerId,
        BitConverter.ToString(StaticData.sessionKey).Replace("-", "").ToLowerInvariant());
```

断开当前账户绑定时调用`bindSession(0L, "")`。Java按认证代次丢弃旧请求回包。
会话仅保存在内存中。每日 Burst Link！未决请求仅将 playerId/UUID 存入 App 私有 preferences；冷启动重新认证后复用 UUID，收到成功结果才清除。旧 add/tool 草稿在升级时清理。

`assets/local-wallet.json` 提供现有服务 origin；复用现有认证和传输：

- `/local-wallet/state`：`{playerId,sessionHex}`，包含 terminal 配置、资格、收入和未读状态。
- `/local-wallet/burst`：另加 `{requestId:UUID}`，服务端返回完整 state 和实际 `burstAwardPoints`。
- `/local-wallet/seen`：另加 `{seenRevision}`，仅在首页取得成功 state 且卡片渲染后确认实际收到的最高收入 revision。
- `/local-wallet/pay`：另加 `{orderId}`；账单继续 `/local-wallet/ledger`，保留筛选、分页和详情。

首页显示超频点数余额、后台动态每日消耗/奖励范围、每日 Burst Link！按钮和纵向到账卡片。注册/Burst 使用固定来源；运营手动/每周投放显示服务端原因。确认已读后，本次收到的卡片仍保留供阅读。全部金额按 100 最小单位 = 1 超频点数显示并去除多余零；终端内待付款金额显示点数，沿用原 amountCents/100 数值；原商品定价存储仍保留。

已认证且 Activity 在前台时每 30 秒低频读取 state；轮询更新红点/状态，不自动确认已读；显式打开首页取得成功 state 后才确认。共用串行请求互斥，账号切换丢弃旧代次回包并清除旧红点；暂停停止调度，销毁移除 callback、断开当前连接。红气泡位于 48dp 浮球窗口内，按半球可见部分夹紧位置。

成功必须 `code:0`，`data` 含现有 `balanceCents/currency:CNY/testOnly:true`。余额不足时提示等待后台投放或参与每日 Burst Link！，确认付款禁用。无限增加余额入口、ADD 页和 `/local-wallet/add` 调用已移除。支付成功继续通知原 `RefreshAfterPayment`，账号设置、IME、拖动与原窗口 token 挂接保持。

## 生成独立 dex

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 tools/local_wallet/build.py --output build/local-wallet-ui-<new-id>
```

复用本机Android33 SDK/JDK21与已装D8 9.4.14，无下载。原build-tools33的旧D8与当前JDK出现NPE，已换到`cmdline-tools/latest/bin/d8`。
Unity stub只进独立编译classpath；D8仅接收钱包class，验证其定义类全在`com.hyjs.wallet`包。
临时class/stub/dex目录自动清理。输出路径必须为新的项目build目录。

## Root 最小接入

1. 从已验证输出取`classes2.dex`及`assets/local-wallet.json`、`assets/local-wallet/icon.png`。
2. 保持原`classes.dex`逐字不变，拒绝覆盖既有同名第二dex；检查新类名无碰撞。
3. 在Root最终选定的manifest XML中，仅把原唯一launcher activity name从`com.unity3d.player.UnityPlayerActivity`替换为`com.hyjs.wallet.WalletActivity`，保留原Unity元数据、横屏、configChanges及本批其他权限。输出里的`AndroidManifest.wallet.xml`是此差量的可审查样例，音频等其他最终标志仍由Root合成。
4. Root原有C#注入桥负责真实认证绑定，原引擎字节保护继续由既有APK验证器核验；该模块不替代原保护。
5. Root 用 `tools/login_ui/build.py --dex-only` 重建含所有现有原生账号类的 classes2.dex；本目录独立 DEX 仅作编译/契约验证，不可直接覆盖当前整合 DEX。
6. 同一个最终 APK 原机核入口/拖动/半球红点、每日 Burst、失败原 UUID 重试、冷登未决恢复、到账卡片/已读竞争、账单/账号设置与指定订单付款。当前批仅 Java/D8 与静态结构通过，HTTP/原机 UI 待 Root 验收。

图标真源`tools/local_wallet/assets/icon.png`由Root图像流程生成，build proof钉其SHA。
当前不触手机、不生成完整APK、不启动服务、不改业务库存。

## 最终原机状态（2026-10-16）

最终整包f5006a54已安装，UI r7仅补48dp浮球自身的系统手势排除。左右普通拖动/半露圆形/闲置透明/打开、金额草稿与冷登88元通过。活动/时间三区和原开服第二天、单项新期刷新已原机通过；后端407cb04已发布，73受影响域与真实MySQL/HTTP整链通过。当前支持30活动类型，6项明确未开放。日期±1/恢复已验；登录奖励仍需原真实重登。

原输入IME适配修改了classes.dex中的四个键盘类，详见`tools/modern_ime/README.md`；r6/r7仅UI升级时继承已验classes.dex/classes3.dex和原managed全部字节。上方“保持原classes.dex”的首次钱包安装步骤适用于68cc基线，不能作为后续IME整包未改原DEX的声明。原机证据入口：`analysis/growth-first-20261016/modern-ime/native-r7-20261016/summary.json`。

## 2026-10-19 拖动窗口修复

浮球改由48dp应用内子窗口承载，保持非聚焦与非模态，不需要系统悬浮权限；登录/注册、钱包与账单Dialog按其token挂接并在关闭后回退。全屏主宿主只提供稳定坐标，拖动移动实际小窗口，停靠半球在小窗口内裁切。

旧Decor手绘r1在真机保持DOWN跨屏时仍不可见，已明确作废；r2原机验证了同一次DOWN经过中央/右上/左下/左上、再次DOWN长拖、半球停靠与闲置透明、纯Unity和原生登录/注册/钱包面板、IME不丢焦点。当前APK只替换classes2.dex，其余10296项压缩载荷、托管程序集与547引擎方法保持。原机未提交登录或注册，已有加密登录缓存字节保持。证据见analysis/wallet-drag-20261019/phone-r2/final.json，打包用本目录package_overlay.py和固定SHA的DEX-only输出。

## 批40验证边界

独立输出 `build/b40-wallet-ui-r1`。verify 核心路由/字段及无限加款路由缺席，并核 DEX 定义类/launcher；这些静态检查不证明 HTTP、冷恢复或原机 UI 已通过。历史原机状态段保留其原日期证据，不能覆盖批40新增路径。
