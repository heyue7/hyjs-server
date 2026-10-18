# 蓝绿钱包 Android 浮窗

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
会话仅保存在内存中；重试加钱所需的非秘密playerId/requestId/amountCents存入本App私有preferences，未决请求在重登后继续用同一UUID。

`assets/local-wallet.json`提供`{"origin":"http://127.0.0.1:8088"}`；复用既有reverse。
三个POST接口：

- `/local-wallet/state`：`{playerId,sessionHex}`。
- `/local-wallet/add`：另加`{requestId:UUID,amountCents}`。
- `/local-wallet/pay`：另加`{orderId}`。

成功必须`code:0`，`data`含`balanceCents/currency:CNY/testOnly:true/entries/pendingOrders`。
界面显示真实服务器余额、收支和待付订单。余额不足时提示增加余额；支付始终确认指定订单，界面不会遍历自动扣全部待付。
未知结果的加钱重试保留原UUID及金额。服务端负责幂等、账户权限、真实原订单支付及归账。
`pay`真实成功且当前认证未变时，Java调用`UnityPlayer.UnitySendMessage("LocalWalletSession", "RefreshAfterPayment", "")`，由Root C#桥请求原SCPlayer刷新钻石HUD；通知不带会话。`add`仅在服务器成功后清除原请求号。

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
5. 同一个最终APK原机核入口可点/拖动/吸边/收起、键盘金额输入、真实加钱和收支、余额不足补钱、指定原订单支付、账号切换和重登。当前仅Java/D8与DEX/manifest结构通过，尚未执行HTTP或原机UI。

图标真源`tools/local_wallet/assets/icon.png`由Root图像流程生成，build proof钉其SHA。
当前不触手机、不生成完整APK、不启动服务、不改业务库存。

## r5 页面与演示工具

- 首页只显示居中余额、增加余额主操作；右上账单进入独立收支页。
- 增加余额进入独立输入页，默认100元；系统数字IME禁用extract/fullscreen，DONE和键盘BACK只收键盘，保留金额/页面。
- 有真实pending订单才显示入口，独立付款页显示真实商品/价格、余额付款方式及确认按钮；不足时转增加余额，完成后返回付款页。
- 底部钱包/活动/时间三tab。活动真实state列逐项canOpen/canRefresh，all/单项apply；时间真实state显示游戏日期和实际日期，previousDay/nextDay/reset均由服务端返回决定。
- 工具请求共用playerId/sessionHex；`/local-play/activities/state|apply`、`/local-play/time/state|change`。apply/change携带UUID，未确认请求按当前player持久化纯业务参数，失败重试沿用原UUID，不保存session。
- 时间state/change成功后经Unity消息`RefreshGameTime(gameTimeMs)`校原客户端时钟，活动apply后另读time/state，并以`RefreshActivities("")`通知Root C#桥刷新原游戏目录。
- 当前r5仅Java/D8及静态契约检查，旧r4原机支付证据不能冒充r5布局/工具原机验收。


## 最终原机状态（2026-10-16）

最终整包f5006a54已安装，UI r7仅补48dp浮球自身的系统手势排除。左右普通拖动/半露圆形/闲置透明/打开、金额草稿与冷登88元通过。活动/时间三区和原开服第二天、单项新期刷新已原机通过；后端407cb04已发布，73受影响域与真实MySQL/HTTP整链通过。当前支持30活动类型，6项明确未开放。日期±1/恢复已验；登录奖励仍需原真实重登。

原输入IME适配修改了classes.dex中的四个键盘类，详见`tools/modern_ime/README.md`；r6/r7仅UI升级时继承已验classes.dex/classes3.dex和原managed全部字节。上方“保持原classes.dex”的首次钱包安装步骤适用于68cc基线，不能作为后续IME整包未改原DEX的声明。原机证据入口：`analysis/growth-first-20261016/modern-ime/native-r7-20261016/summary.json`。
