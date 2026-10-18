# 全背包十倍容量的 VIP 扩容预览

用户最新决定将所有背包容量统一放大十倍，替代此前角色包五倍决定。实际容量读取服务端 `warriorBagSize/equipBagSize/petBagSize`；原扩容弹窗另用 `TableVipPerks.numPerk3/numPerk4` 差额预测角色/装备容量，对应四处分支差额同步乘十。

只修改两个原方法：`UI.BagPage.OnCapacityClick` 的两个角色包差额和两个装备包差额，以及 `VIPData.InitVipDes` 用来格式化角色/装备额外容量权益文案的两个整数；各插入 `ldc.i4.s 10; mul`。原 VIP 上限、下一级/跳级选择、确认购买委托、价格和其他权益保持逐条原指令。原数据表不改，避免其他逻辑重复乘。原精灵包没有这一弹窗分支或独立 VIP 扩容预览/权益，不新造入口；其实际十倍容量由服务端返回。

```sh
rtk proxy .analysis-venv/bin/python tools/client_capacity_ui/build.py --input-managed PATH --input-managed-sha256 SHA --output-directory analysis/unique-capacity-overlay
```

工具接受前一步明确 SHA 的 DLL，保留每个其他原方法、字段和 ABI 并输出 manifest。只生成 DLL overlay，不安装或部署。

输入可为 hero-r2 或顺序 weapon 等已固定输出，但两方法必须仍是原方法 SHA `a3341c98...` / `3b12e606...`。构建前拒绝叠加旧五倍或已乘十的产物，避免乘50/100。验证同时完整比较原 Assembly/所有 AssemblyRef 的名称、版本、公钥/标志及CLI/强命名状态，拒绝空公钥被标为完整公钥。`capacity-preview-overlay-r1` 与仅弹窗的 `capacity-preview-10x-overlay-r2` 均已作废；最终使用 r3 或根最新同源组合。
