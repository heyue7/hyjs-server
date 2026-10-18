# 原语音本地 PCM 适配

本工具只构建独立 managed 桥和 `Assembly-CSharp.dll` 副本；APK 打包、原设备录音和安装由主线程执行。默认 `Enabled=false`，服务端 `p10_voice_policies.enabled=false`，原云认证/私有格式/ASR未恢复。

1. 使用 `.analysis-venv/bin/python tools/p10_voice/build.py --input <当前受控 Assembly-CSharp.dll> --output-directory <空目录> --origin <明确本地 HTTP/HTTPS origin>` 构建默认关闭副本。只在明确启用客户端适配时传 `--enable-local-voice`。原登录 `audiosystem` key 必须由实际 Voice policy 推导，原 `audioautoget` 保持具名本地 off。
2. 将输出 `LocalVoicePCM.dll` 与修改副本一起加入 managed 目录，保留原运行库。工具核对全部原方法身份，除10个 VoicePlugin 入口外保留26936原方法语义及异常区域；全部 `Battle.Engine` 原指令字节精确保持。TypeSpec/MemberRef重复别名可由原写入器归并，只允许其解析后语义相同。
3. 原 `CallbackData`/`UI.ChatBase.SendRecord` 继续发送 `isAudio=2`。Microphone 真实单声道16k采样经 `AudioClip.GetData` 转成 PCM16 WAV；原30秒上限/操作间隔1秒、实际AudioTime向下取整、字节精确FileSize。服务先验完整WAV并存InnoDB，客户端核SHA与真实长度后才回callback(true)。无假转写文本。
4. 每次开始播放均重新认证 POST `/voice/download`。没有磁盘音频缓存，没有预取成功票据绕过当前会话/黑名单/会员/场次。原 `DeleFile` 仅本地历史回收语义，桥无缓存可回收；它不会删除另一人的服务端音频。取消尚未发送且已返回的本人上传可请求 `/voice/delete`，无法删除的超时上传由真实TTL清blob。
5. `ClearRecordDelage`、暂停/换账号取消待回调，停止录放；队列保留原max10，每项到播放边界重新下载。认证失败不得播放旧音频。编译、合成PCM传输及权限测试不能替代原手机麦克风和扬声器验收。

运行过程不写出或打印登录session、原AppKey或openid。服务边界检查见 root 的独立 HTTP handler，当前输入有正角色ID、16bytes Base64当前会话、实际scope/nonce、PCM WAV硬上限和真实opaque filename。
