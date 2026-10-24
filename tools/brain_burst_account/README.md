# Brain Burst 2039 账号客户端覆盖

账号页管理名称、密码和退出，角色页进入各区服蓝绿终端。登录采用原账号/选服/创角协议；`LocalWalletSession.LogoutAccount` 在服务端确认退出后调用原 `ManagerController.BackToLogin`。登录缓存、在途响应与原充值预检查 ABI 保留。

`build.py` 显式绑定 APK SHA，仅重编译既有两个 helper 和原生 DEX，原 `Assembly-CSharp.dll` 保持字节一致。图标来源与完整 image_gen 提示词在 `tools/local_wallet/assets/brain-burst-source.json`、`brain-burst.prompt.txt`。火焰 B 为低清参考的生成修复像素。

```sh
rtk proxy .analysis-venv/bin/python tools/brain_burst_account/build.py \
  --base-apk build/hyjs-local.apk --base-sha256 <已核对SHA> \
  --output-directory analysis/<新的覆盖目录>
rtk proxy .analysis-venv/bin/python tools/client_release/package_native.py \
  --base-apk build/hyjs-local.apk --overlay analysis/<新的覆盖目录> \
  --output apks/build/<新的候选>.apk --record analysis/<新的打包凭证>.json
```

构建和打包不安装、不修改 current、不发布。实际验证和未验边界见 `plans/20261025-brain-burst-2039.md`。
