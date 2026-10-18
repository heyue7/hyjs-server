# 从客户端恢复的传输格式

证据：`analysis/protocol_methods.il.txt` 中的 `NetWork.Packets.Factory.PacketFactory` 方法；TCP 接收长度规则另见 `analysis/selected_methods.il.txt` 中 `SocketTool.ReceiveHandler`。

所有包头整数使用大端；Protobuf 消息体使用自身标准编码。

## HTTP 请求

`POST /sao/`，二进制请求体。`HttpUtil.Send` 使用 `WebClient.UploadDataAsync`。

| 偏移 | 字节数 | 含义 |
| --- | --- | --- |
| 0 | 4 | 包总长度，包含 68 字节头 |
| 4 | 4 | 协议版本 10000 |
| 8 | 4 | 消息编号 |
| 12 | 8 | playerId |
| 20 | 4 | zoneId |
| 24 | 4 | Protobuf 体长度 |
| 28 | 16 | sessionKey；未登录时为 ASCII 空格加 15 个零字节 |
| 44 | 16 | 客户端 MD5 签名原始字节 |
| 60 | 8 | 毫秒时间戳 |
| 68 | N | Protobuf 体 |

签名：`MD5(ASCII(decimal(playerId) + decimal(zoneId) + decimal(timestamp) + 客户端固定后缀) + protobufBody)`。
固定后缀从 `PacketFactory.GetSig` 恢复，见实现。签名只证明符合旧包格式，不能代替会话认证。

## HTTP 响应与 TCP 响应

两者均为 `int32 bodyLength + int32 opcode + protobufBody`。HTTP 允许连续拼接多条消息，首条作为响应，其余交给客户端推送处理。

特别注意：TCP 响应的长度字段只计 Protobuf 体。客户端先读 4 字节长度，再读取 `bodyLength + 4` 字节（消息编号和消息体）。此处与 TCP 请求不同。

## TCP 请求

| 偏移 | 字节数 | 含义 |
| --- | --- | --- |
| 0 | 4 | 其后全部长度，即 `24 + bodyLength` |
| 4 | 4 | 协议版本 1 |
| 8 | 4 | 消息编号 |
| 12 | 4 | zoneId |
| 16 | 8 | playerId |
| 24 | 4 | Protobuf 体长度 |
| 28 | N | Protobuf 体 |

`CSSocketLogin` 以消息体中的 sessionKey 验证会话；`NetAPI.SCLoginTcpCallback` 将 `SCSocketLogin.result == 100` 视为成功。

## 证据边界

包结构和消息映射已通过静态 IL/元数据确定，服务端实现已用本机网络测试。尚未使用原客户端产生请求并消费响应，也没有旧服务器抓包对照。业务数值与功能仍需逐条重建。
