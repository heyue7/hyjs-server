# 王者招募：开放世界选择与三原角色卡面

输入已验r7 APK：`eba29af9e52db8b2af751f568ef51f1a98e52611457c16720d2ce4ecbd0389f7`；原main：`e4e577fe401920ef3f17ced0f48d761d74437bca42d2e4fd2b6db15d4f230689`。`pins.json`固定9个实际方法。REA parent Evidence `ev_9d17605d9c4176785dc3008b413046b30e36b27b6482aa1cc271d24a1022d700`绑定该SHA/MVID；完整原IL在`analysis/play-feedback-20261019/recruit-shop/king-ui-il.txt`。

- 页面从原游戏会话查询当前角色当前开放世界；只显示开放项。切换成功后重新走原招募信息回包，原动作直到选中世界回包到达才放行。原抽取请求发出后直到其成功/失败回调到达都阻止切世界；退出/换号关闭世界查询的在途请求；不会发放资源。
- 抽取、预览、幸运领奖仍走原costType3及原opcode。每日50次/幸运500/+10/十抽保底/48h免费冷却共用原存档。
- 凪hero069、罗亚070、克洛维073的原`hero2d`不存在。仅在原`RecruitResultCardItem.SetHeroBuyId`把这三条请求指向各自已存在的原`heropainter/hero_card_XXX`，沿既有HeroArt裁切，保原预制尺寸/alpha。无新增绘图。
- `build.py`只生成managed/helper+manifest，不打包、不安装。可用`--input-managed`叠其他代理修改，但9个目标方法须仍匹配；547原引擎方法和其余原方法raw bodies不变，原ABI/token不变。

```
rtk proxy .analysis-venv/bin/python tools/client_king_recruit/build.py \
 --base-apk apks/build/hyjs-dialog-final-r7.apk \
 --base-sha256 eba29af9e52db8b2af751f568ef51f1a98e52611457c16720d2ce4ecbd0389f7 \
 --input-managed /absolute/path/to/predecessor/Assembly-CSharp.dll \
 --output-directory /absolute/path/to/new-output
```

产物需同时替换`Assembly-CSharp.dll`并新增`SoulFlowerKingRecruit.dll`，保留输入APK的`SoulFlowerHeroArt.dll`。先部署带`InitializeKingRecruitWorlds`、两鉴权接口与world snapshot支持的服务端。原机布局/世界选择/三图仍由root合批验收，静态校验不代表上机通过。
