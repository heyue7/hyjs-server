# 原游戏账号面板修复

保留 `Pages/loginUI` 的背景、面板、输入框、关闭和登录流程。按钮复用原包公会金色按钮像素，来源与精确裁切见 `resources/source.json`；没有使用生成图片。注册复用同一面板，增加确认密码，调用现有账号注册接口；成功后返回手动登录。记住密码继续使用原加密缓存，默认凭据不指向验收账号时禁止直接登录。

`build.py` 为可组合覆盖：只改 UserInput 的 Show/OnRegistButtonClick/OnLoginButton，修正客服 DoOpen 的 maxStack，并去除 WorldMapPage.transfer 错误的两处 town_side_ 前缀。其余原方法逐字节验证，547 个原战斗方法保持。Java 改动由 login_ui 的 DEX 构建合成，既有公开 ABI 保持。

本轮原机验收和最终制品以 `plans/27-play-feedback-20261020.md` 为准，早期 r1/r2/r4 仅为问题定位候选。
