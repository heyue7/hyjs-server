package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestNativeApplicationPauseValidation(t *testing.T) {
	// 原 protobuf 默认值代表恢复；未知状态在协议边界拒绝。
	for _, state := range []int32{-1, 0, 1, 2} {
		err := ValidateGameRequest(&model.GamePacket{Body: &pb.CSEnterBackgroud{Exit: proto.Int32(state)}})
		if (err == nil) != (state == 0 || state == 1) {
			t.Fatalf("pause state %d accepted=%v", state, err == nil)
		}
	}
	if err := ValidateGameRequest(&model.GamePacket{Body: &pb.CSEnterBackgroud{}}); err != nil {
		t.Fatal(err)
	}
}
