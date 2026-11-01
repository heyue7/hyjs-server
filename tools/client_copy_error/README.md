# 主线战斗业务错误提示离线补丁

本工具绑定 1.1.5 候选包及原 `Assembly-CSharp.dll` 的 SHA-256。它仅修改 `Copy.CopyManager` 的主线 `CSCopyFightStart` 回调：保留原特效和遮罩清理；普通业务 `ErrorMessage` 已由 HTTP 层展示时，跳过重复的 1047 强制重登。会话错误 21、不可解析的错误码、空包及传输/协议失败继续原路径。原 HTTP 层对 21 本身直接要求重登，本补丁不改变它。

使用已有的本机 .NET 和 dnlib，输出必须是不存在的目录：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_copy_error/build.py \
  --base-apk '/Users/lirui/LR/hyjs/deliveries/client-update-1.1.5/黑衣剑士灵魂之花v1.1.5-采集恢复候选.apk' \
  --base-proof /Users/lirui/LR/hyjs/deliveries/client-update-1.1.5/package-record.json \
  --toolchain-root /Users/lirui/LR/hyjs/.tools/native-batch \
  --output-directory /tmp/hyjs-copy-error-overlay
```

校验要求原包及包记录、目标回调和 HTTP 相关方法的准确摘要；新程序集的类型、字段和方法 ABI、MVID、所有其他方法原始头/IL/EH、原回调已有指令和跳转、战斗引擎均须保持。脚本解释执行新增 IL 的业务错误、会话码、无效码与空包分支；该模型不执行 Unity，也不能代替真机验收。输出只含单个程序集及证明，不打包、不安装、不部署。不得将输出当作已验证的客户端包。
