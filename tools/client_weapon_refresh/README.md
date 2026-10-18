# 武器转换后的原列表刷新

原 `EquipRemodelPage` 成功将同一武器 GUID 装给目标角色，更新 `owner/lastHeroId` 并播放转换动画。原详情页的 `onEquipCallBack` 未传入转换页；动画关闭后补一次当前可见原装备列表的刷新，避免保留转换前缓存图标。

仅挂接原空方法 `EquipRemodelPage.DoClose`，延后两帧等待 `PageManager` 恢复下层页面。阵容页调用原 `EquipTab.UpdateTab`，背包按原 `curType/curEquipInnerTab` 重建图标；他人阵容不处理。角色和装备的所有权、数量、费用和已穿戴分类继续由原模型/原方法决定。

```sh
rtk proxy .analysis-venv/bin/python tools/client_weapon_refresh/build.py --output-directory analysis/unique-weapon-overlay
```

顺序组合其他已固定补丁时，再传入 `--input-managed PATH --input-managed-sha256 SHA`。原 APK 仍锁定 SHA `34ce7e0d...`，提供原 Unity/mscorlib 依赖。工具只输出小 DLL overlay 和静态证明，不打包、安装或部署。

根代理合包须同时替换 `Assembly-CSharp.dll` 并新增 `SoulFlowerWeaponRefresh.dll`。静态检查会保证其余原方法/ABI/字段和 547 个战斗方法完全保留；真实刷新效果仍需原机确认。
