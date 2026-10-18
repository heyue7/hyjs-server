# 决赛原 UI 最小提案

此目录只有提案源码，没有构建、改包、安装或手机操作。Root统一核输入SHA、组合当前补丁、构建签名和实机。

原证据：`analysis/p14/cross-server-original.il.txt`4013–4048的`UI.ServerBattleFinals.InitData`首层列表无条件`firstList[idx]`，第二/三层有空列表分支；`analysis/p14/finals-player-ui-original.il.txt`原`InitPlayerInfo(null,...)`会隐藏按钮/排行并展示原空位。原`isOwner`只比较pid，InitPlayerInfo在调用前已经写入完整`randItem`。

仅允许两个变化：

- `UI.ServerBattleFinals.InitData`首层取值前检查真实列表非null且`Count>idx`，缺位传null；保留全部真实数据、原控件和后两层逻辑，不制造空Proto玩家。
- `UI.ServerBattlePlayerUIItem.isOwner`先验证`randItem.serverId==StaticData.playerData.ZoneId`，随后执行原pid比较；无randItem返回原false分支。

Injector沿当前项目dnlib/PreserveAll方式，只插入局部CIL，没有新增方法/字段，无异常区域修改。遇不匹配的原指令形状立即失败。Root验证器allowed_entries精确为`{('ServerBattleFinals','InitData'),('ServerBattlePlayerUIItem','isOwner')}`；仍需逐方法/547引擎原字节及全部EH校验。实际原4人/同pid异服、各group真实空位/原英雄点击/type3回包由Root单机验收；本提案不包含任何战绩、轮空晋级或多局胜负。

Root独立构建命令：使用项目`.analysis-venv/bin/python`运行本目录`build.py --input <当前已核SHA的Assembly-CSharp.dll> --input-sha256 <精确64hex> --output-directory <独立空目录>`。仅输出DLL和manifest；临时编译项目在成功/失败时清理，失败移除本次DLL/proof。未执行此命令，当前无DLL产物。验证同时要求547原engine方法、全部未改方法CIL/EH、目标两方法原空EH、无新增方法/类型。
