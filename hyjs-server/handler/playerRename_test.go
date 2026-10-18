package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestPlayerRenameOriginalRequestWidthValidationPreservesInput(t *testing.T) {
	for _, name := range []string{"Alice0123456", "六个汉字正好", "ĀĀĀĀĀĀĀĀĀĀĀĀ"} {
		body := &pb.CSChangePlayerName{PlayerName: proto.String(name)}
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: body}); err != nil {
			t.Fatal("original width rejected", name, err)
		}
		if body.GetPlayerName() != name {
			t.Fatal("native callback nickname identity was silently normalized")
		}
	}
	for _, name := range []string{"", " ", " Alice", "Alice ", "1234567890123", "七个汉字超限了", "āāāāāāā", "甲\n乙", "甲\u200b乙", string([]byte{0xff})} {
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSChangePlayerName{PlayerName: proto.String(name)}}); err == nil {
			t.Fatal("invalid original rename", name)
		}
	}
	for _, stamp := range []int64{0, -1} {
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: stamp, Body: &pb.CSChangePlayerName{PlayerName: proto.String("合法昵称")}}); err == nil {
			t.Fatal("missing mutation timestamp", stamp)
		}
	}
}
