package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP7BrokenPlaybackUsesFrozenRushResultOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   int32
		manual bool
	}{
		{"trial-manual", constUtil.P7RushTrial, true},
		{"elite-auto", constUtil.P7RushElite, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, startRequest := p7ScopedFixture(t, tc.kind)
			if err := db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			if tc.manual {
				if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
					t.Fatal(err)
				}
			}
			before, _ := f.profile(t)
			start, err := f.s.p7Start(f.ctx, f.id, startRequest, 30501, false)
			if err != nil {
				t.Fatal(err)
			}
			var terminal model.CombatFinalHPOutcome
			var previewBytes []byte
			if tc.manual {
				var row db.P7RushManualCalculation
				if err = db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil || json.Unmarshal(row.Preview, &terminal) != nil {
					t.Fatal("missing frozen preview", err)
				}
				previewBytes = append([]byte(nil), row.Preview...)
			} else {
				var row db.P7RushScopedCalculation
				if err = db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil || json.Unmarshal(row.Terminal, &terminal) != nil {
					t.Fatal("missing frozen terminal", err)
				}
			}
			won := int32(0)
			if terminal.Outcome.Winner == 0 {
				won = 1
			}
			end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1 - won)}
			if tc.manual {
				end.OptRecord = proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
				end.OptRecord.AttackerTotalLeftHp = proto.Float64(end.OptRecord.GetAttackerTotalLeftHp() + 1)
			}
			if _, err = f.s.p7End(f.ctx, f.id+1, end); err == nil {
				t.Fatal("foreign role settled fight")
			}
			if tc.manual {
				broken := append(append([]byte(nil), previewBytes...), 'x')
				if err = db.GetIns().Model(&db.P7RushManualCalculation{}).Where("fight_id=?", start.GetFightId()).UpdateColumn("preview", broken).Error; err != nil {
					t.Fatal(err)
				}
				if _, err = f.s.p7End(f.ctx, f.id, end); err == nil {
					t.Fatal("corrupt server calculation accepted")
				}
				if err = db.GetIns().Model(&db.P7RushManualCalculation{}).Where("fight_id=?", start.GetFightId()).UpdateColumn("preview", previewBytes).Error; err != nil {
					t.Fatal(err)
				}
			}
			replies, err := f.s.p7End(f.ctx, f.id, end)
			if err != nil || replies[0].(*pb.SCRushFightEnd).GetResult() != won {
				t.Fatal("server result not settled", err)
			}
			after, game := f.profile(t)
			expected := before.GetPoint()
			if won == 1 {
				expected += 10
			}
			if after.GetPoint() != expected {
				t.Fatal("reward did not follow server result")
			}
			// A damaged playback after acceptance must not invalidate the receipt.
			changedReplay := db.GetIns().Model(&db.P6BattleRecord{}).Where("player_id=? AND source=? AND source_fight=?", f.id, constUtil.P7RushReplaySource, start.GetFightId()).Updates(map[string]any{"operates": []byte("broken"), "replay_hash": "broken", "client_result": 17, "server_result": 17})
			if changedReplay.Error != nil || changedReplay.RowsAffected != 1 {
				t.Fatal("could not damage isolated replay", changedReplay.Error)
			}
			f.restart(t)
			retry, err := f.s.p7End(f.ctx, f.id, end)
			if err != nil || !proto.Equal(replies[0], retry[0]) {
				t.Fatal("cold retry changed result", err)
			}
			again, gameAgain := f.profile(t)
			if !proto.Equal(after, again) || !p6CopySeriesJSONEqual(t, game, gameAgain) {
				t.Fatal("cold retry duplicated reward")
			}
			changed := proto.Clone(end).(*pb.CSRushFightEnd)
			changed.Result = proto.Int32(won)
			if _, err = f.s.p7End(f.ctx, f.id, changed); err == nil {
				t.Fatal("different retry replaced accepted receipt")
			}
		})
	}
}

func TestP7BrokenPlaybackUsesFrozenAwakeResultOnce(t *testing.T) {
	f := p7AwakeBusinessFixture(t)
	var third *pb.Warrior
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		if _, err := f.s.GrantFixedReward(p, g, "hero_011,1"); err != nil {
			return err
		}
		for _, hero := range g.Warriors {
			if hero.GetId() == "hero_011" {
				third = proto.Clone(hero).(*pb.Warrior)
			}
		}
		return nil
	}); err != nil || third == nil {
		t.Fatal("third isolated actor", err)
	}
	if _, err := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: third.Id, Guid: third.Guid, FormatPos: proto.Int32(3)}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	enter := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	begin, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: enter.Monsters[0].Guid}, now)
	if err != nil {
		t.Fatal(err)
	}
	start := begin[0].(*pb.SCAwakeCopyFightBegin)
	var fight db.P7AwakeFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	input, _, err := p7AwakeFightInput(db.GetIns().WithContext(f.ctx), &fight)
	if err != nil {
		t.Fatal(err)
	}
	server, err := f.s.p7Awake16ServerFallback(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	won := int32(0)
	if server.Outcome.Winner == 0 {
		won = 1
	}
	end := &pb.CSAwakeCopyFightEnd{FightId: start.FightId, IsWinClient: proto.Int32(1 - won)}
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id+1, end, now); err == nil {
		t.Fatal("foreign role settled fight")
	}
	replies, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || replies[0].(*pb.SCAwakeCopyFightEnd).GetIsWinServer() != won {
		t.Fatal("server result not settled", err)
	}
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var saved model.CombatFinalHPOutcome
	if json.Unmarshal(fight.Terminal, &saved) != nil || saved.Outcome.CapabilityHash != constUtil.P7Awake16ServerFallbackProof || saved.Outcome.Winner != server.Outcome.Winner {
		t.Fatal("fallback source not saved")
	}
	after, game := f.profile(t)
	changedReplay := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=? AND player_id=?", fight.ReplayID, f.id).Updates(map[string]any{"operates": []byte("broken"), "replay_hash": "broken", "client_result": 17, "server_result": 17})
	if changedReplay.Error != nil || changedReplay.RowsAffected != 1 {
		t.Fatal("could not damage isolated replay", changedReplay.Error)
	}
	f.restart(t)
	retry, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || !proto.Equal(replies[0], retry[0]) {
		t.Fatal("cold retry changed result", err)
	}
	again, gameAgain := f.profile(t)
	if !proto.Equal(after, again) || !p6CopySeriesJSONEqual(t, game, gameAgain) {
		t.Fatal("cold retry duplicated reward")
	}
	changed := proto.Clone(end).(*pb.CSAwakeCopyFightEnd)
	changed.IsWinClient = proto.Int32(won)
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, changed, now); err == nil {
		t.Fatal("different retry replaced accepted receipt")
	}
	if bytes.Equal(fight.EndRequest, nil) {
		t.Fatal("request identity was not retained")
	}
}
