# 原游戏账号面板修复

保留 `Pages/loginUI` 的背景、面板、输入框、关闭和登录流程。按用户 2026-10-20 的刀剑神域风格反馈，去掉早期候选的公会金色按钮，使用浅灰/青蓝细边按钮与小型复选框。`resources/sao-v2/` 保存完整 imagegen 提示词、实际游戏参考设计、透明按钮图集、来源与 UV 取样坐标；图集由内置 image_gen 生成，明确区别于原包像素。按钮标题由原游戏字体实时渲染，普通/按下状态共用图集；复选框为原生 NGUI 几何，可点选整行文字。

注册复用同一面板，增加确认密码，调用现有账号注册接口；成功后返回手动登录。记住密码继续使用原加密缓存，默认凭据不指向验收账号时禁止直接登录。

`build.py` 为可组合覆盖：只改 UserInput 的 Show/OnRegistButtonClick/OnLoginButton，修正客服 DoOpen 的 maxStack，并去除 WorldMapPage.transfer 错误的两处 town_side_ 前缀。其余原方法逐字节验证，547 个原战斗方法保持。Java 改动由 login_ui 的 DEX 构建合成，既有公开 ABI 保持。

本轮原机验收和最终制品以 `plans/27-play-feedback-20261020.md` 为准，早期 r1/r2/r4 仅为问题定位候选。
