package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

// The original limited-sign tab shares its reward UI with daily sign-in. Its
// two requests must pass through the earlier P11 dispatcher into existing P4.
func TestP4LimitedSignOriginalRequestsReachExistingWelfare(t *testing.T) {
	f := p4Setup(t)
	for index, request := range []proto.Message{&pb.CSRewardLimitSignList{}, &pb.CSReceiveLimitMonthSign{}} {
		if reply, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, int64(index+1)); handled || err != nil || reply != nil {
			t.Fatal("original daily-sign request was intercepted by limited arena", request, handled, err)
		}
		reply, handled, err := f.s.DispatchP4(f.ctx, f.id, request, int64(index+1))
		if err != nil || !handled || len(reply) != 1 {
			t.Fatal("existing P4 welfare producer was unreachable", handled, err)
		}
		if index == 0 {
			data := reply[0].(*pb.SCRewardLimitSignList).GetLimitDaySignDatas()
			if data.GetAllNum() != 7 || data.GetSignNum() != 0 || len(data.List) != 7 || data.List[0].GetGetState() != 1 || data.List[0].GetRewardId() != "coin,1000" {
				t.Fatal("existing local seven-day rule changed", data)
			}
		} else {
			gain := reply[0].(*pb.SCReceiveLimitMonthSign).GetGet()
			if len(gain.GetSimps()) != 1 || gain.Simps[0].GetId() != "coin" || gain.Simps[0].GetNum() != 1000 {
				t.Fatal("existing first daily reward changed", gain)
			}
		}
	}
	player, _ := f.profile(t)
	if player.GetCoin() != 1000 {
		t.Fatal("actual existing welfare grant missing", player.GetCoin())
	}
}
