package service

import (
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

// These are actual paid start objects encoded into the original protocol. The
// independent socket bytes were not captured; the test preserves that boundary.
func TestP6ActualPaidChapter3CopySeriesSourceAndStrictAutomatic(t *testing.T) {
	s := p6OriginalTables(t)
	for _, item := range []struct{ task, protoSHA, jsonSHA string }{
		{"10", "5bf9b6c128da80c9b0b9812e5b60456d583b866c22d329b40f3704b0bf20ccae", "cb94cf48ae7facf8b250ccbdb1fd8da5291ffb8d95b2e0fd11cc83ad05ac2890"},
		{"12", "502cc6a0765aee667797bb8fd56c9ad4060afc39b406d3e24958de7b54e25d01", "28d8052507cd1f6d8fc56fc437b1c4cc601ca8710cfd03d6e3eb4e2bfc9ef04d"},
	} {
		t.Run(item.task, func(t *testing.T) {
			prefix := "../../analysis/p15/actual-copy-starts-20261015/mission3-task" + item.task + ".SCCopyFightStart"
			raw, err := os.ReadFile(prefix + ".pb")
			if err != nil || p6Digest(raw) != item.protoSHA {
				t.Fatal("actual paid start proto representation SHA", err)
			}
			start := &pb.SCCopyFightStart{}
			if err = proto.Unmarshal(raw, start); err != nil {
				t.Fatal(err)
			}
			raw, err = os.ReadFile(prefix + ".source.json")
			if err != nil || p6Digest(raw) != item.jsonSHA {
				t.Fatal("actual paid source object SHA", err)
			}
			object := &pb.SCCopyFightStart{}
			if err = json.Unmarshal(raw, object); err != nil || !proto.Equal(start, object) || start.GetBattlePlayer().GetPlayerId() != 4 || len(start.GetBattleMonsterList()) != 1 {
				t.Fatal("source object/protocol semantic identity or finite single-wave count", err)
			}
			// Explicit automatic validation is read-only and supplies no server winner.
			frozen, err := s.FreezeP6CopySeriesInput(start, true, map[string][]string{"source": {"actual ID4 paid chapter3 task" + item.task + " start object; finite strict automatic read-only validation; socket bytes not independently captured"}}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.ComputeP6CopySeries(t.Context(), frozen, nil)
			if err != nil || len(out.Fights) != 1 || out.Authoritative || out.Fights[0].Outcome.PVECompatibilityRule != "" {
				t.Fatal("actual paid strict automatic series calculation", err)
			}
			t.Logf("actual paid task%s strict computed winner=%d rounds=%d RNG=%d; original fresh headless numeric comparison pending", item.task, out.Winner, out.Fights[0].Outcome.FinalRound, out.Fights[0].Outcome.RandomDraws)
		})
	}
}
