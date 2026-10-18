package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP11ArenaGuideOriginalWireAndFirstOrderBoundary(t *testing.T) {
	for _, pair := range [][2]proto.Message{
		{&pb.CSPvpFindMatch{}, &pb.CSPvpFindMatchGuide{}},
		{&pb.SCPvpFindMatch{}, &pb.SCPvpFindMatchGuide{}},
		{&pb.CSPvpFightResult{}, &pb.CSPvpFightResultGuide{}},
		{&pb.SCPvpFightResult{}, &pb.SCPvpFightResultGuide{}},
	} {
		a, b := pair[0].ProtoReflect().Descriptor().Fields(), pair[1].ProtoReflect().Descriptor().Fields()
		if a.Len() != b.Len() {
			t.Fatal("original Guide field count", a.Len(), b.Len())
		}
		for i := 0; i < a.Len(); i++ {
			x, y := a.Get(i), b.Get(i)
			if x.Name() != y.Name() || x.Number() != y.Number() || x.Kind() != y.Kind() || x.Cardinality() != y.Cardinality() || x.Message() != y.Message() {
				t.Fatal("original Guide field wire shape differs", x, y)
			}
		}
	}
	ctx := context.WithValue(context.Background(), constUtil.P11ArenaGuideContextKey{}, int64(7))
	if p11ArenaGuideSourceLabel(ctx) != constUtil.P11ArenaGuideSource || p11ArenaGuideSourceLabel(context.Background()) != constUtil.P11ArenaModernSource {
		t.Fatal("actual Guide request source lost original23105 identity")
	}
	p := &pb.Player{PlayerId: proto.Int64(7), Level: proto.Int32(10), NoviceGuideSchedule: proto.Int32(12)}
	a := &pb.BattlePlayer{PlayerId: proto.Int64(7), Inst: &pb.BattlePlayerInst{FirstOrder: proto.Int32(1)}, Formation: []*pb.BattleHero{{Guid: proto.String("actual-main"), SkillOrder: proto.String("047_1,047_2,047_3")}}}
	d := &pb.BattlePlayer{PlayerId: proto.Int64(8), Inst: &pb.BattlePlayerInst{FirstOrder: proto.Int32(0)}, Formation: []*pb.BattleHero{{Guid: proto.String("actual-peer"), SkillOrder: proto.String("047_1,047_2,047_3")}}}
	if err := p11ArenaGuidePrepare(ctx, p, a, d, false); err != nil {
		t.Fatal(err)
	}
	a.Inst.FirstOrder, d.Inst.FirstOrder = proto.Int32(0), proto.Int32(1)
	if err := p11ArenaGuidePrepare(ctx, p, a, d, false); err == nil {
		t.Fatal("Guide forced attacker-first accepted modern defender-first")
	}
	if err := p11ArenaGuidePrepare(context.Background(), nil, nil, nil, true); err != nil {
		t.Fatal("ordinary Arena altered by Guide hook", err)
	}
	if err := p11ArenaGuidePrepare(ctx, p, nil, nil, true); err != nil || p.GetNoviceGuideSchedule() != 13 {
		t.Fatal("actual accepted completion missing", err)
	}
	if err := p11ArenaGuidePrepare(ctx, p, nil, nil, false); err == nil {
		t.Fatal("completed guide admitted fresh request")
	}
}

func TestP11ArenaGuideRealPeersActualCostRankProgressAndCold(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("independent type2 source not certified")
	}
	f, peer := p11ArenaScopedSetup(t)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(12)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	list, err := f.s.p11ArenaGuideAt(f.ctx, f.id, &pb.CSPvpFindMatchGuide{}, 79101, now)
	if err != nil || len(list) != 1 || len(list[0].(*pb.SCPvpFindMatchGuide).GetRankList()) < 2 {
		t.Fatal("original Guide two real peers", list, err)
	}
	before, _ := p10LuckyProfile(t, f.id)
	request := &pb.CSPvpFightResultGuide{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	result, err := f.s.p11ArenaGuideAt(f.ctx, f.id, request, 79102, now)
	if err != nil || len(result) != 2 {
		t.Fatal("original Guide paid settlement", result, err)
	}
	response := result[0].(*pb.SCPvpFightResultGuide)
	if response.GetFightId() == "" || response.GetMaxRoundNum() != 40 || len(response.GetPlayerList()) != 2 || response.GetPlayerList()[0].GetInst().GetFirstOrder() != 1 || response.GetPlayerList()[1].GetInst().GetFirstOrder() != 0 {
		t.Fatal("original Guide complete source changed", response)
	}
	current, game := p10LuckyProfile(t, f.id)
	if current.GetEnergy() != before.GetEnergy()-2 || current.GetNoviceGuideSchedule() != 13 || current.GetHeroWarId() != before.GetHeroWarId() {
		t.Fatal("sameTX Guide cost/progress/main", current)
	}
	if result[1].(*pb.SCPlayer).GetPlayer().GetNoviceGuideSchedule() != 13 {
		t.Fatal("original current player lost completion")
	}
	if _, err := f.s.p11ArenaGuideAt(f.ctx, f.id, request, 79103, now); err == nil {
		t.Fatal("fresh completed Guide charged twice")
	}
	raw, _ := proto.Marshal(response)
	for i := 0; i < 2; i++ {
		if i == 1 {
			sql, _ := db.GetIns().DB()
			sql.Close()
			f.open(t)
		}
		replay, err := f.s.p11ArenaGuideAt(f.ctx, f.id, request, 79102, now)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := proto.Marshal(replay[0])
		if !bytes.Equal(raw, body) {
			t.Fatal("Guide receipt response bytes changed")
		}
		p, g := p10LuckyProfile(t, f.id)
		if !proto.Equal(p, current) || len(g.Warriors) != len(game.Warriors) {
			t.Fatal("Guide cold/retry mutated current profile")
		}
	}
}
