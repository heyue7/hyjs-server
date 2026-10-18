package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestFormationAnyHeroOffRequiresOneRemainingAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		_, err := f.s.GrantFixedReward(p, g, "hero_020,1;equip_001,1;trea_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	main, err := p5Warrior(g, p.GetHeroWarId())
	if err != nil {
		t.Fatal(err)
	}
	var partner *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_020" {
			partner = w
		}
	}
	if partner == nil {
		t.Fatal("fixture partner missing")
	}
	equip := g.Equipment[0]
	treasure := g.Treasures[0]
	if _, err := f.s.equipmentFormationOn(f.ctx, f.id, &pb.CSFormationOn{Id: equip.EquipId, Guid: equip.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.treasureFormationOn(f.ctx, f.id, &pb.CSFormationOn{Id: treasure.TreasureId, Guid: treasure.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal(err)
	}
	mainOff := &pb.CSFormationOff{Id: main.Id, Guid: main.Guid, FormatPos: proto.Int32(1)}
	if _, err := f.s.formationOff(f.ctx, f.id, mainOff); err != nil {
		t.Fatal("main off with another hero", err)
	}
	beforeP, beforeG := f.profile(t)
	if beforeP.GetHeroWarId() != main.GetGuid() || formationHero(beforeG.Formation[0]) != nil || len(beforeG.Formation[0].Things) != 0 {
		t.Fatal("identity changed or removed slot retained ghost loadout")
	}
	unequipped, err := f.s.equipmentRecord(beforeG, equip.GetGuid())
	if err != nil || unequipped.GetLastHeroId() != "" || len(beforeG.Equipment) != len(g.Equipment) || len(beforeG.Treasures) != len(g.Treasures) {
		t.Fatal("auto detach lost inventory or kept equipment owner", err)
	}
	if err := P5GuardInstance(beforeP, beforeG, equip.GetGuid()); err != nil {
		t.Fatal("auto-detached equipment retained ghost guard", err)
	}
	partnerOff := &pb.CSFormationOff{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.formationOff(f.ctx, f.id, partnerOff); err == nil || err.Error() != constUtil.FormationRequiresHero {
		t.Fatal("last companion removal accepted", err)
	}
	afterP, afterG := f.profile(t)
	if !proto.Equal(beforeP, afterP) || !proto.Equal(formationListResult(beforeG), formationListResult(afterG)) {
		t.Fatal("failed last-member removal changed profile")
	}
	if _, err := f.s.formationOff(f.ctx, f.id, mainOff); err != nil {
		t.Fatal("same unloaded hero retry", err)
	}
	f.restart(t)
	out, err := f.s.formationList(f.ctx, f.id)
	if err != nil || len(out[0].(*pb.SCFormationList).GetFormationList()) != 1 || formationHero(out[0].(*pb.SCFormationList).FormationList[0]).GetGuid() != partner.GetGuid() {
		t.Fatal("cold companion-only formation", err)
	}
	// 冷登后自动卸回的实例可以直接穿到剩余队友，保留原GUID和培养。
	if _, err := f.s.equipmentFormationOn(f.ctx, f.id, &pb.CSFormationOn{Id: equip.EquipId, Guid: equip.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal("cold re-equip automatically detached equipment", err)
	}
	if _, err := f.s.treasureFormationOn(f.ctx, f.id, &pb.CSFormationOn{Id: treasure.TreasureId, Guid: treasure.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal("cold re-equip automatically detached treasure", err)
	}
	// 单人阵容原位替换保持一人，主角仍可以用自己的原实例返回。
	if _, err := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: main.Id, Guid: main.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal("single-member replacement", err)
	}
	if _, err := f.s.formationOff(f.ctx, f.id, &pb.CSFormationOff{Id: main.Id, Guid: main.Guid, FormatPos: proto.Int32(2)}); err == nil {
		t.Fatal("last main removal accepted")
	}
	// 同时卸下两人时，存档行锁必须让第二个请求看到只剩一人。
	if _, err := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, request := range []*pb.CSFormationOff{{Id: main.Id, Guid: main.Guid, FormatPos: proto.Int32(2)}, {Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(1)}} {
		go func(request *pb.CSFormationOff) {
			<-start
			_, err := f.s.formationOff(f.ctx, f.id, request)
			results <- err
		}(request)
	}
	close(start)
	var successes int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if err.Error() != constUtil.FormationRequiresHero {
			t.Fatal("concurrent off error", err)
		}
	}
	_, final := f.profile(t)
	if successes != 1 || len(formationListResult(final).GetFormationList()) != 1 || len(final.Equipment) != len(g.Equipment) || len(final.Treasures) != len(g.Treasures) {
		t.Fatal("concurrent removal lost last member or inventory")
	}
}

func TestCopyBoxEnteredSceneBeforeBattleRejectsForeignAndCorrupt(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		g.Run = &model.CopyRun{Chapter: "1", Mission: "3"}
		_, err := f.s.CaptureCopyBoxes(tx, f.id, "1", "3")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSCopyBox{BoxId: proto.String("box_3")}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Run.Mission = "2"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.copyBoxOpen(f.ctx, f.id, request, 101); err == nil {
		t.Fatal("foreign scene chest")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Run.Mission = "3"; return nil }); err != nil {
		t.Fatal(err)
	}
	var row db.CopyBoxClaim
	if err := db.GetIns().First(&row, "player_id=? AND box_id=?", f.id, "box_3").Error; err != nil {
		t.Fatal(err)
	}
	originalHash := row.InputHash
	if err := db.GetIns().Model(&row).Update("input_hash", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.copyBoxOpen(f.ctx, f.id, request, 101); err == nil {
		t.Fatal("corrupt chest freeze")
	}
	if err := db.GetIns().Model(&row).Update("input_hash", originalHash).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	out, err := f.s.copyBoxOpen(f.ctx, f.id, request, 101)
	if err != nil || len(out[0].(*pb.SCCopyBox).GetCurrentResource()) != 2 {
		t.Fatal("scene touch before first battle", err)
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 80000 || after.GetDiamond()-before.GetDiamond() != 50 {
		t.Fatal("wrong frozen original reward")
	}
	if _, err := f.s.copyBoxOpen(f.ctx, f.id, request, 102); err == nil {
		t.Fatal("distinct repeat grants")
	}
}

func TestChapterRewardGuideOnlyReconcilesCompletedOriginalStep(t *testing.T) {
	for _, tc := range []struct {
		name           string
		schedule       int32
		quest, claimed bool
		chapter        string
		want           bool
	}{
		{"early-claim", 9, true, true, "1", true},
		{"not-claimed", 9, true, false, "1", false},
		{"wrong-chapter", 9, true, true, "2", false},
		{"before-quest", 8, false, true, "1", false},
		{"quest-missing", 9, false, true, "1", false},
		{"later-equipment", 11, true, true, "1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pb.Player{NoviceGuideSchedule: proto.Int32(tc.schedule)}
			g := &model.GameState{Copy: &pb.SCCopyPassedInfo{PassedChapterInfo: []*pb.CopyChapterInfo{{ChapterId: proto.String(tc.chapter), Reward0Claimed: proto.Bool(tc.claimed)}}}}
			if tc.quest {
				g.Quests = []string{"quest_5"}
			}
			if got := ReconcileChapterRewardGuide(p, g); got != tc.want {
				t.Fatal("guide state", got)
			}
			want := tc.schedule
			if tc.want {
				want = 10
			}
			if p.GetNoviceGuideSchedule() != want {
				t.Fatal("unrelated guide step changed")
			}
		})
	}
}

func TestChapterRewardEarlyClaimThenOriginalQuestAndNormalOrder(t *testing.T) {
	for _, early := range []bool{true, false} {
		name := "quest-first"
		if early {
			name = "reward-first"
		}
		t.Run(name, func(t *testing.T) {
			f := storyQuestReceiptSetup(t, "quest_5")
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
				p.NoviceGuideSchedule = proto.Int32(8)
				p.EquipBagSize = proto.Int32(60)
				for _, mission := range g.Copy.GetPassedCopyInfo() {
					if mission.GetChapterId() == "1" && mission.GetMissionId() == "1" {
						mission.Star = proto.Int32(5)
					}
				}
				f.s.refreshCopyProgress(g)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			award := &pb.CSCopyGetStarAward{ChapterId: proto.String("1"), StarIndex: proto.Int32(0)}
			claim := func() {
				t.Helper()
				if _, err := f.s.chapterStarReward(f.ctx, f.id, award); err != nil {
					t.Fatal(err)
				}
			}
			if early {
				claim()
				p, _ := f.profile(t)
				if p.GetNoviceGuideSchedule() != 8 {
					t.Fatal("early award skipped unfinished quest guide")
				}
			}
			out, err := f.s.storyQuestReward(f.ctx, f.id, "quest_5", 301)
			if err != nil {
				t.Fatal(err)
			}
			want := int32(9)
			if early {
				want = 10
			}
			if out[1].(*pb.SCPlayer).GetPlayer().GetNoviceGuideSchedule() != want {
				t.Fatal("quest response has stale guide state")
			}
			if !early {
				claim()
			}
			p, g := f.profile(t)
			if p.GetNoviceGuideSchedule() != 10 {
				t.Fatal("original equip guide was not resumed")
			}
			var claimed bool
			for _, chapter := range g.Copy.PassedChapterInfo {
				if chapter.GetChapterId() == "1" {
					claimed = chapter.GetReward0Claimed()
				}
			}
			if !claimed {
				t.Fatal("actual chapter reward missing")
			}
			if _, err := f.s.chapterStarReward(f.ctx, f.id, award); err == nil {
				t.Fatal("guide recovery duplicated chapter reward")
			}
			f.restart(t)
			cold, _ := f.profile(t)
			if cold.GetNoviceGuideSchedule() != 10 || cold.GetCoin() != p.GetCoin() || cold.GetDiamond() != p.GetDiamond() {
				t.Fatal("cold guide or reward drift")
			}
		})
	}
}
