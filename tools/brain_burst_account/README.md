# Brain Burst 2039 账号客户端覆盖

账号页管理名称、密码和退出，角色页进入各区服蓝绿终端。登录先通过 `/game-account/login` 真实验证全局账号，再沿原选服/20009/创角协议进入角色；`LocalWalletSession.LogoutAccount` 在服务端确认退出后调用原 `ManagerController.BackToLogin`。登录缓存、在途响应与原充值预检查 ABI 保留。

`build.py` 显式绑定 APK SHA，仅重编译既有两个 helper 和原生 DEX，默认保留原 `Assembly-CSharp.dll` 字节；显式传 `--sign-redpoint-overlay` 时再次严格验证并组合两个签到成功回调的20117刷新，其余26944方法及547战斗方法保持。图标来源与完整 image_gen 提示词在 `tools/local_wallet/assets/brain-burst-source.json`、`brain-burst.prompt.txt`。火焰 B 为低清参考的生成修复像素。

```sh
rtk proxy .analysis-venv/bin/python tools/brain_burst_account/build.py \
  --base-apk build/hyjs-local.apk --base-sha256 <已核对SHA> \
  --output-directory analysis/<新的覆盖目录> \
  --sign-redpoint-overlay analysis/<已固定的签到覆盖>
rtk proxy .analysis-venv/bin/python tools/client_release/package_native.py \
  --base-apk build/hyjs-local.apk --overlay analysis/<新的覆盖目录> \
  --output apks/build/<新的候选>.apk --record analysis/<新的打包凭证>.json
```

构建和打包不安装、不修改 current、不发布。实际验证和未验边界见 `plans/67-brain-burst-2039-20261025.md`。

## 2026-10-25 账号界面精修

账号首页与名称/密码表单采用原登录灰白细边和青蓝按钮；按钮读取 `assets/brain-burst/button-atlas.png` 的 normal/pressed 主次裁块，首次加载缓存，缺失时使用同色 native gradient。旧 `login-ui/panel.png` 不参与此页。

账号窗横屏最多640dp、窄屏最多440dp，保留32dp屏幕边距与90%可用高度上限；首页310dp，44dp标题栏右上关闭。身份与角色在滚动正文中；修改名称、修改密码、退出登录三等宽44dp操作固定底部。当前角色以明确的“蓝绿终端”按钮进入，其他服继续显示进入该服后的真实状态，无新增跳服。

名称/密码保留EditText、单行密码、IME与原校验、提交锁及服务器错误；同页重绘保存已有密码输入，离开表单清除输入引用。宽窗新密码与确认并列，窄窗纵向滚动。表单说明缩为“不会更改角色名”与“其他设备需用新密码重新登录”。钱包HOME、账单与收银台布局及全部网络/凭据逻辑保持。

执行代理仅完成轻量Java检查；同状态原机前后对照、键盘/失败重试、退出重登和素材打包由主代理验收，未据编译结果宣称视觉通过。

## 68期全局账号与所有终端页面

`GameAccountLogin` 使用受尝试代次保护的异步账号登录，成功后提交内存64位全局token及公用资料，不创建或旋转角色。客户端仅在真实登录成功后保存用户勾选的加密凭据；改密后同步当前内存密码，进区服继续原20009。资料/名称/密码/退出优先使用全局token，并在已有角色时同时提交原角色凭据；钱包、账单、Burst和付款继续只用角色凭据。两级generation丢弃切号后的旧响应；退出/切号清除可见账号、角色和钱包状态。

账号、钱包、账单、到账、规则、订单和付款统一原游戏灰白细边与atlas按钮；帮助采用圈i点击同款弹窗。账单收支对照收紧以露出首条流水；付款商品/金额/收款角色/余额/扣点后果与确认同组，订单号在圈i参考信息里。默认详情完成按钮160dp，宽窗详情字段同排、窄窗纵排滚动。原订单ID、支付确认、幂等和忙碌锁保留。

同账号、同数据、同手机视口的67/68前后对照与实际交互在 `analysis/brain-burst68-20261025/`，本批范围和待确认奖历见 `plans/68-account-signin-arena-20261025.md`。67期段落记录当期行为，当前实现以上段为准。
