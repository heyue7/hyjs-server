package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

// TestP13GuildIngress 原HTTP/TCP共用字段校验；非BMP字符按原UTF16长度计数。
func TestP13GuildIngressOriginalEnumsLengthsAndTargets(t *testing.T) {
	valid := []proto.Message{
		&pb.CSUnionSearch{Page: proto.Int32(1), Name: proto.String("32ae04c1-abcd-4123-b341-000000000001")},
		&pb.CSUnionSearch{Page: proto.Int32(1), Name: proto.String("32AE04C1-ABCD-4123-B341-000000000001")},
		&pb.CSUnionCreate{Name: proto.String("真实公会")}, &pb.CSUnionCreate{Name: proto.String(strings.Repeat("剑", 6))},
		&pb.CSUnionSearch{Page: proto.Int32(1)}, &pb.CSUnionApply{UnionId: proto.String("actual-guild")}, &pb.CSUnionUnApply{UnionId: proto.String("actual-guild")},
		&pb.CSUnionAgreeApply{PlayerId: proto.Int64(2)}, &pb.CSUnionRefuseApply{PlayerId: proto.Int64(2)}, &pb.CSPromotionViceChairman{PlayerId: proto.Int64(2)}, &pb.CSDismissViceChairman{PlayerId: proto.Int64(2)}, &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(2)}, &pb.CSTransferChairman{PlayerId: proto.Int64(2)},
		&pb.CSUnionNoticeChange{Content: proto.String(strings.Repeat("公", 70))}, &pb.CSUnionDeclarationChange{Content: proto.String(strings.Repeat("宣", 40))}, &pb.CSChangeUnionFlag{UnionFlag: proto.Int32(0)}, &pb.CSUnionDonate{DonateType: proto.String("coin")}, &pb.CSUnionDonate{DonateType: proto.String("badge")}, &pb.CSQuitUnion{}, &pb.CSDissolveUnion{}, &pb.CSUnionUpgrade{}, &pb.CSGetUnionStamina{},
	}
	for _, req := range valid {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: req}); err != nil {
			t.Fatalf("valid %T: %v", req, err)
		}
	}
	for _, name := range []string{strings.Repeat("A", 12), strings.Repeat("Ā", 12), strings.Repeat("ā", 6), strings.Repeat("😀", 3), "ABCDEF公会名"} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: &pb.CSUnionCreate{Name: proto.String(name)}}); err != nil {
			t.Fatal("原加权名称预算边界", name, err)
		}
	}
	for _, request := range []proto.Message{&pb.CSUnionCreate{Name: proto.String(strings.Repeat("剑", 7))}, &pb.CSUnionCreate{Name: proto.String(strings.Repeat("Ā", 13))}, &pb.CSUnionCreate{Name: proto.String(strings.Repeat("ā", 7))}, &pb.CSUnionCreate{Name: proto.String(strings.Repeat("😀", 4))}, &pb.CSUnionNoticeChange{Content: proto.String(strings.Repeat("公", 71))}, &pb.CSUnionDeclarationChange{Content: proto.String(strings.Repeat("宣", 41))}} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: request}); err == nil {
			t.Fatalf("原加权预算超限 %T", request)
		}
	}
	invalid := []proto.Message{
		&pb.CSUnionSearch{Page: proto.Int32(1), Name: proto.String("32ae04c1-abcd-4123-b341-00000000000z")},
		&pb.CSUnionCreate{}, &pb.CSUnionCreate{Name: proto.String("   ")}, &pb.CSUnionCreate{Name: proto.String(strings.Repeat("剑", 13))}, &pb.CSUnionCreate{Name: proto.String(strings.Repeat("😀", 7))},
		&pb.CSUnionSearch{Page: proto.Int32(0)}, &pb.CSUnionSearch{Page: proto.Int32(-1)}, &pb.CSUnionSearch{Page: proto.Int32(100001)}, &pb.CSUnionSearch{Page: proto.Int32(1), Name: proto.String(strings.Repeat("😀", 7))},
		&pb.CSUnionApply{}, &pb.CSUnionApply{UnionId: proto.String(" guild ")}, &pb.CSUnionUnApply{UnionId: proto.String(strings.Repeat("a", 65))},
		&pb.CSUnionAgreeApply{PlayerId: proto.Int64(0)}, &pb.CSUnionRefuseApply{PlayerId: proto.Int64(1)}, &pb.CSPromotionViceChairman{PlayerId: proto.Int64(1)}, &pb.CSDismissViceChairman{PlayerId: proto.Int64(1)}, &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(1)}, &pb.CSTransferChairman{PlayerId: proto.Int64(-1)},
		&pb.CSUnionNoticeChange{Content: proto.String(strings.Repeat("公", 141))}, &pb.CSUnionDeclarationChange{Content: proto.String(strings.Repeat("宣", 81))}, &pb.CSChangeUnionFlag{UnionFlag: proto.Int32(-1)}, &pb.CSUnionDonate{DonateType: proto.String("diamond")},
	}
	for _, req := range invalid {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: req}); err == nil {
			t.Fatalf("invalid %T accepted", req)
		}
	}
	for _, req := range valid {
		switch req.(type) {
		case *pb.CSUnionSearch:
			continue
		}
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 0, Body: req}); err == nil {
			t.Fatalf("missing timestamp %T", req)
		}
	}
}
