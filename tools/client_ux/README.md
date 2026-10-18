# 原客户端引导、公告与客服修复

当前候选：`build/hyjs-client-ux-candidate.apk`，SHA `a5ecbcd89f6aaa21a123166208fa66d6e9a4bf593f9f0333ce7dcd872ef018d3`。构建证据位于 `analysis/client-ux-20261018/apk-build-r1.json`，最终覆盖为 `overlay-r5`。原机状态以本批 `phone` 记录为准，静态通过不代替原机验证。

`Injector.cs` 只改七个明确方法：

- 从原 `10056262.apk` 恢复 `GuideManager.OnLogin/OnLevelUp/OnQuestComplete`。原指令、控制流、异常区域、局部签名与栈上限逐项核对；未清玩家引导存档，未删除现有固定招募兼容。
- 在原 `UI.AnnouncePage.DoOpen`、`SystemAnnouncePage.Show` 前加公告透明处理，保留原框体、布局、标题和关闭逻辑；Java 仅处理 URL 为 `/notices` 的既有 SDK WebView。
- 原 `UI.KeFuPage.DoOpen` 保留原 Page 打开，再展示灰绿底、蓝标题的客服交互；关闭时回原页面管理器。
- 原成就成功回包为空时，用原 `BoxManager` 提示“成就奖励待配置”，不生成原创奖励。

攻略错误的客户端原调用已定位到 `UI.UICopyBossCook.RefreshBossCook` 与 `UI.WantedMission.Show`，均发送 `CSGetRebels`。该项由服务端为低等级聚合查询返回空目录处理，客户端未添加旁路。

`ClientFeedback.java` 使用现有内存桥已验证的 `player/session/generation`，仅向真实反馈 JSON API 发请求，会话变化时自动关闭。包含列表、新建、详情、继续补充及刷新客服回复；幂等 UUID 对同一未确认内容重试保持。没有密码、会话文件或 URL 会话参数。

公告 HTML 的唯一服务端差量位于 `hyjs-server/handler/adminHandler.go` 的 `PublicAnnouncementsHandler`：透明底、蓝标题黄线、灰褐正文，原转义及安全头保持。客户端源码恢复后，文件已归还主代理接公共路由。

`build.py` 与 `package.py` 只替换 DLL 和 `classes2.dex`。全部其他 10288 个继承条目的内容 SHA、压缩 payload 与尺寸校验保持；原 547 个战斗方法、IME、品牌、钱包、注册与资源刷新继承。最终结果不修改 current 或 rollback，由主代理按原机证据提升。

失败和清理记录：`first-failures.json`、`owned-intermediate-cleanup.json`。早期 r1–r4 的自有重复 DLL/Dex 已清理，保留失败日志、历史 manifest 与最终 r5；不碰共享依赖或其他历史候选。
