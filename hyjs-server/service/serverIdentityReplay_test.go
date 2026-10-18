package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	pb "hyjs-server/model/protocol"
	"testing"
)

// TestTrustedLocalReplayServer 验证主键与原别名在真实部署区服中沿用同一归属边界。
func TestTrustedLocalReplayServer(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	s.ServerID = 2
	var recordID string
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		var err error
		recordID, err = s.CaptureBattleReplay(ctx, tx, id, "trusted-local", "owned-alias", *input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err := s.ComputeFrozenBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		return s.FinishObservedBattleReplay(ctx, tx, id, "trusted-local", "owned-alias", outcome.Operates, outcome.Winner == 0)
	}); err != nil {
		t.Fatal(err)
	}
	var first proto.Message
	for _, key := range []string{recordID, "owned-alias"} {
		response, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(key), ServerId: proto.Int32(2)})
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = response[0]
		} else if !proto.Equal(first, response[0]) {
			t.Fatal("primary/alias replay differs")
		}
		if _, err := s.fightPlayback(ctx, id+1, &pb.CSFightPlayback{FightId: proto.String(key), ServerId: proto.Int32(2)}); err == nil {
			t.Fatal("foreign owner replay accepted")
		}
		if _, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(key), ServerId: proto.Int32(1)}); err == nil {
			t.Fatal("foreign zone replay accepted")
		}
	}
}
