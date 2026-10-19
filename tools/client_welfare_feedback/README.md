# 原活动/成就反馈覆盖层

输入任意已组合 Assembly-CSharp.dll，仅校验并修改两个原方法：

- `UI.AcRechargeSingleItem::GetTextString`：原 type32 攻略福利回空标题。添加 type32 原目标格式“主线通关{0}次”，保留其余类型原字符串、奖励及领取请求。
- `AchievementItem::SetReciveState`：原领取状态设置完成后，按现有按钮状态稳定重排 SimpleGrid，优先可领取项、待完成项、已领取项。不重建列表、不复发奖励、保留滚动位置。

```sh
rtk proxy .analysis-venv/bin/python tools/client_welfare_feedback/build.py \
  --input-managed /absolute/composed/Assembly-CSharp.dll \
  --base-apk apks/build/hyjs-dialog-final-r7.apk \
  --output-directory analysis/fresh-output
```

独立 helper `SoulFlowerWelfareFeedback.dll` 使用原 Unity profile/C#3。保留原 TypeDef/Field/MethodDef、字段/方法token与ABI、MVID和已有AssemblyRef，除两个白名单目标之外逐raw方法核对，547个Battle.Engine方法保持。当前候选见 `analysis/play-feedback-20261019/welfare/feedback-r1/manifest.json`。静态验证与原机验证分别记录。
