# 原领奖回调后的权威红点查询

原RewardCenterPage的“一键领取”成功回调清空当前可见列表并把rewardCenter置0；单件领取成功后RemoveAndUpdate在可见列表空时同样置0。服务端若仍有下一批附件，或打开列表后又收到新邮件，先到的TCP权威红点可被这些本地回调覆盖。

本工具只在上述两条成功路径完成原本地清理后发送原20117 CSRedPoint socket查询。沿用原20118订阅与原RedPointTip消费者。失败回调仍跳原ret，不发送额外查询。领奖请求、奖励内容、原成功提示及本地清理顺序保持；没有新增helper程序集，没有直接伪清/伪设服务端红点。

```sh
rtk proxy .analysis-venv/bin/python tools/client_reward_redpoint/build.py \
  --input-managed analysis/client-achievements-20261019/candidate-r1/Assembly-CSharp.dll \
  --output-directory analysis/client-reward-redpoint-20261019/candidate-r1
rtk proxy .analysis-venv/bin/python tools/client_reward_redpoint/verify_representatives.py \
  --managed analysis/client-reward-redpoint-20261019/candidate-r1/Assembly-CSharp.dll \
  --output analysis/client-reward-redpoint-20261019/candidate-r1/representatives.json
```

支持串行组合任意显式输入DLL，仅两个目标方法的原raw-body SHA必须符合target-pins；其他overlay方法在该次输入/输出间全部逐字校验保留，547个原引擎方法不变，方法/字段token和ABI保持。产物只有Assembly-CSharp.dll及manifest/日志。root统一与成就和福利覆盖层组合并打包，工具不复制整APK、不签名、不安装、不访问服务器。

representatives执行实际产物CIL的成功、失败、单件部分领取和最后一条可见领取分支，外部API使用显式stub。该证据验证查询位于本地清理之后，不代表Unity、真实网络时序或原机已验证。
