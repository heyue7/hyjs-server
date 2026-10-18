package model

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestOriginalPacketLayout(t *testing.T) {
	raw, err := hex.DecodeString("0000004f0000271000004e25000000000000007b000000010000000b2020000000000000000000000000000013459aa42cb2d0af804d7a77ecd3cfa00000018bcfe568000a04746573741203726573")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseGamePacket(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	body := p.Body.(*pb.CSCheckVersion)
	if p.PlayerId != 123 || p.ZoneId != 1 || p.Timestamp != 1700000000000 || body.GetAppVersion() != "test" || body.GetResVersoin() != "res" {
		t.Fatal("client layout changed")
	}
	for _, bad := range [][]byte{raw[:20], raw[:len(raw)-1], append(append([]byte{}, raw...), 0)} {
		if _, err := ParseGamePacket(bad, false); err == nil {
			t.Fatal("accepted invalid length")
		}
	}
	raw[44] ^= 1
	if _, err := ParseGamePacket(raw, false); err == nil {
		t.Fatal("accepted invalid signature")
	}
}

func TestPacketRejectsHeaderMismatchAndMissingRequiredBody(t *testing.T) {
	body, err := proto.Marshal(&pb.CSSocketLogin{SessionKey: make([]byte, 16)})
	if err != nil {
		t.Fatal(err)
	}
	var opcode int32
	for code, name := range OpcodeNames {
		if name == "CSSocketLogin" {
			opcode = code
			break
		}
	}
	for _, tcp := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "tcp"}[tcp], func(t *testing.T) {
			header := 68
			if tcp {
				header = 28
			}
			makePacket := func(payload []byte) []byte {
				raw := make([]byte, header, header+len(payload))
				binary.BigEndian.PutUint32(raw, uint32(header+len(payload)))
				binary.BigEndian.PutUint32(raw[4:], 10000)
				binary.BigEndian.PutUint32(raw[8:], uint32(opcode))
				binary.BigEndian.PutUint32(raw[24:], uint32(len(payload)))
				if tcp {
					binary.BigEndian.PutUint32(raw, uint32(header-4+len(payload)))
					binary.BigEndian.PutUint32(raw[4:], 1)
				} else {
					// 保持 APK 对十进制头字段与原始 Protobuf 体的签名规则。
					sig := md5.New()
					fmt.Fprintf(sig, "000%s", constUtil.SigningSuffix)
					sig.Write(payload)
					copy(raw[44:60], sig.Sum(nil))
				}
				return append(raw, payload...)
			}
			raw := makePacket(body)
			if _, err = ParseGamePacket(raw, tcp); err != nil {
				t.Fatal(err)
			}
			for _, offset := range []int{0, 4, 24} {
				bad := append([]byte(nil), raw...)
				bad[offset+3] ^= 1
				if _, err = ParseGamePacket(bad, tcp); err == nil {
					t.Fatalf("accepted header mismatch at byte %d", offset)
				}
			}
			badOpcode := append([]byte(nil), raw...)
			binary.BigEndian.PutUint32(badOpcode[8:], 0xffffffff)
			if _, err = ParseGamePacket(badOpcode, tcp); err == nil {
				t.Fatal("accepted unknown opcode")
			}
			if _, err = ParseGamePacket(makePacket(nil), tcp); err == nil {
				t.Fatal("accepted missing required protobuf fields")
			}
			if !tcp {
				for _, offset := range []int{12, 20, 60, header} {
					bad := append([]byte(nil), raw...)
					bad[offset] ^= 1
					if _, err = ParseGamePacket(bad, false); err == nil {
						t.Fatalf("accepted modified signed data at byte %d", offset)
					}
				}
			}
		})
	}
}

func TestHTTPResponseConcatenationPreservesEachBodyLength(t *testing.T) {
	messages := []proto.Message{
		&pb.SCSocketLogin{Result: proto.Int32(100)},
		&pb.ErrorMessage{Code: proto.String(constUtil.ErrGameSession), Msg: proto.String("invalid session")},
	}
	raw, err := EncodeGameResponses(messages)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range messages {
		if len(raw) < 8 {
			t.Fatal("truncated response prefix")
		}
		size := int(binary.BigEndian.Uint32(raw))
		if size > len(raw)-8 {
			t.Fatal("response length included an unrelated frame")
		}
		name := OpcodeNames[int32(binary.BigEndian.Uint32(raw[4:]))]
		got, err := GameMessage(name, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = proto.Unmarshal(raw[8:8+size], got); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(got, want) {
			t.Fatal("concatenated frame changed message contents")
		}
		raw = raw[8+size:]
	}
	if len(raw) != 0 {
		t.Fatal("unexpected response suffix")
	}
}

func TestTCPResponseLengthAndRequiredFields(t *testing.T) {
	raw, err := EncodeGameResponses([]proto.Message{&pb.SCSocketLogin{Result: proto.Int32(100)}})
	if err != nil || hex.EncodeToString(raw) != "0000000200004e400864" {
		t.Fatalf("invalid original response: %x %v", raw, err)
	}
	if _, err = EncodeGameResponses([]proto.Message{&pb.SCSocketLogin{}}); err == nil {
		t.Fatal("accepted missing required result")
	}
}

func TestGameMessageRejectsProtocolTypos(t *testing.T) {
	if _, err := GameMessage("BattleHero", map[string]any{"missingField": 1}); err == nil {
		t.Fatal("silently discarded unknown field")
	}
}
