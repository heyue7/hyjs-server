# 宠物装备反馈48修复

只接管 `UI.PetEquipAwakeTab.SetCost` 和 `UI.PetEquipStrengthTab.isCanStrength`，增加 `SoulFlowerPetEquip48.dll`。觉醒按下一行 cost/selfCostNum 显示原图标和数量，供体规则对齐服务端 p9BasicDonor，强化5次允许最终不足5阶按实际剩余阶数执行。现有 GetFiveCost 边界报价、余额不足整批拒绝、0级初始装备、全强化轮转、回执与消费协议均保留。

客户端供体只取同名非目标、owner为空、level1/exp0/突破0/亲密等级与经验0、已有装备培养均0的实例；selfCostNum=0不显示宠物消耗。材料库存使用long比较、int钳制展示，复用原iconPref/CommonIcon/UIGrid。没有新增商品、价格、自动补装或投放。

从仓库运行，依赖主目录现有 .analysis-venv 与 .tools：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_pet_equip48/build.py \
  --runtime-apk /Users/lirui/LR/hyjs/build/hyjs-local.apk \
  --base-sha256 3bc0d36520bd298029fd9bc437b48af7015ce819cfc0f1e1275746e4718d3bac \
  --tool-root /Users/lirui/LR/hyjs \
  --output-directory /path/to/fresh-output
```

合并其他overlay可传 `--input-managed /path/to/Assembly-CSharp.dll`；两个目标方法必须仍匹配固定基线原始header/IL/EH SHA，其余已有方法严格保留。输出目录必须新建。工具按 C#3/原APK引用编译helper，逐方法核ABI/token/flags、所有非白名单原始header/IL/EH及547个Engine方法逐字不变；编译临时目录自动回收。输出manifest和两个DLL交主代理打包，工具不打APK、不安装、不连DB或设备。

静态编译/方法不变证明不代替原Unity界面验收；主代理验证 selfCostNum=0/1/2、错误供体/已守护/培养状态、材料不足、最终剩余1–4级和满级无扣费。
