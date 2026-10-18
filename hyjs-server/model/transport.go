package model

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"hyjs-server/constUtil"
	"strings"

	"google.golang.org/protobuf/proto"
)

// ParseGamePacket 校验原客户端包长、版本和签名后解码 Protobuf。
func ParseGamePacket(raw []byte, tcp bool) (*GamePacket, error) {
	header := 68
	if tcp {
		header = 28
	}
	if len(raw) < header || len(raw) > constUtil.MaxPacket {
		return nil, errors.New("Invalid packet size")
	}
	u32 := func(i int) int32 { return int32(binary.BigEndian.Uint32(raw[i : i+4])) }
	p := &GamePacket{Opcode: u32(8)}
	if tcp {
		if u32(0) != int32(len(raw)-4) || u32(4) != 1 || u32(24) != int32(len(raw)-header) {
			return nil, errors.New("TCP header mismatch")
		}
		p.ZoneId = u32(12)
		p.PlayerId = int64(binary.BigEndian.Uint64(raw[16:24]))
	} else {
		if u32(0) != int32(len(raw)) || u32(4) != 10000 || u32(24) != int32(len(raw)-header) {
			return nil, errors.New("HTTP header mismatch")
		}
		p.PlayerId = int64(binary.BigEndian.Uint64(raw[12:20]))
		p.ZoneId = u32(20)
		p.Session = append([]byte(nil), raw[28:44]...)
		p.Timestamp = int64(binary.BigEndian.Uint64(raw[60:68]))
		sig := md5.New()
		fmt.Fprintf(sig, "%d%d%d%s", p.PlayerId, p.ZoneId, p.Timestamp, constUtil.SigningSuffix)
		sig.Write(raw[header:])
		if subtle.ConstantTimeCompare(sig.Sum(nil), raw[44:60]) != 1 {
			return nil, errors.New("Invalid packet signature")
		}
	}
	name := OpcodeNames[p.Opcode]
	if !strings.HasPrefix(name, "CS") {
		return nil, errors.New("Unknown request opcode")
	}
	msg, err := GameMessage(name, nil)
	if err != nil {
		return nil, err
	}
	if err = proto.Unmarshal(raw[header:], msg); err != nil {
		return nil, errors.New("Invalid protobuf body")
	}
	p.Body = msg
	return p, nil
}

// EncodeGameResponses 保持 HTTP 多消息拼接和 TCP 的 bodyLength 长度规则。
func EncodeGameResponses(messages []proto.Message) ([]byte, error) {
	var out []byte
	for _, msg := range messages {
		if msg == nil {
			return nil, errors.New("Nil response")
		}
		name := string(msg.ProtoReflect().Descriptor().Name())
		var opcode int32
		for code, n := range OpcodeNames {
			if n == name {
				opcode = code
				break
			}
		}
		if opcode == 0 {
			return nil, fmt.Errorf("Unknown response %s", name)
		}
		payload, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
		if len(payload) > constUtil.MaxPacket-8 {
			return nil, errors.New("Response too large")
		}
		out = binary.BigEndian.AppendUint32(out, uint32(len(payload)))
		out = binary.BigEndian.AppendUint32(out, uint32(opcode))
		out = append(out, payload...)
	}
	return out, nil
}
