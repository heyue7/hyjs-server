package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 仅导入已捕获的已装备队伍前提，新号普通库存/钱包和全部P7进度不从历史导入。
func p7PartyBusinessFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7SectionFixture(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready, f.s.P9PetsReady = true, true
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "p7-party-"+fmt.Sprint(time.Now().UnixNano()), "isolated-party-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "历史队伍前提", "hero_001"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	raw, err := readTestAnalysis75("../../analysis/p7/party-rush-isolated-roster-r2-20261016.json")
	if err != nil || p6Digest(raw) != "e60e9774a43efe58f67b9eb84c1a30cf40a3a30f2e5f68c87d063d3aeb29a5c9" {
		t.Fatal("finite historical equipped roster SHA", err)
	}
	var source struct {
		Player struct {
			Level int32  `json:"level"`
			Main  string `json:"heroWarId"`
		} `json:"playerPatch"`
		Game struct {
			Warriors  []*pb.Warrior       `json:"warriors"`
			Formation []*pb.FormationItem `json:"formation"`
			Equipment []*pb.Equipment     `json:"equipment"`
		} `json:"gamePatch"`
		Pet struct {
			GUID string `json:"guid"`
			ID   string `json:"id"`
			Wire string `json:"wireHex"`
		} `json:"pet"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Game.Warriors) != 2 || len(source.Game.Equipment) != 1 || source.Pet.ID != "pet_002" {
		t.Fatal("finite actual roster shape")
	}
	before, items := f.profile(t)
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.HeroWarId = proto.Int32(source.Player.Level), proto.String(source.Player.Main)
		g.Warriors, g.Formation, g.Equipment = source.Game.Warriors, source.Game.Formation, source.Game.Equipment
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	petRaw, err := hex.DecodeString(source.Pet.Wire)
	if err != nil {
		t.Fatal(err)
	}
	pet := &pb.Pet{}
	if proto.Unmarshal(petRaw, pet) != nil || pet.GetGuid() != source.Pet.GUID || pet.GetId() != source.Pet.ID {
		t.Fatal("historical actual pet bytes")
	}
	if err = db.GetIns().Create(&db.P9PetInstance{GUID: pet.GetGuid(), PlayerID: f.id, PetID: pet.GetId(), Data: petRaw, CreatedAt: time.Now().UnixMilli(), Source: "finite captured equipped roster only; source SHAe60e9774; no natural acquisition claim"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Where("guid=? AND player_id=?", pet.GetGuid(), f.id).Delete(&db.P9PetInstance{}) })
	_, withoutProjection := f.profile(t)
	if _, _, _, missing := f.s.P9GuardProperties(withoutProjection, withoutProjection.Formation[0]); missing == nil {
		t.Fatal("finite owned Pet alone unexpectedly supplied its unpublished guard snapshot")
	}
	// 原守护入口从必要拥有实例发布p9.pet快照；不复制历史普通KV或回执。
	if _, err = f.s.p9GuardOn(f.ctx, f.id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 20790); err != nil {
		t.Fatal("publish actual historical owned Pet through original guard action", err)
	}
	after, current := f.profile(t)
	projectedRaw, err := base64.StdEncoding.DecodeString(p5KV(current, "p9.pet:"+pet.GetGuid()))
	projected := &pb.Pet{}
	if err != nil || proto.Unmarshal(projectedRaw, projected) != nil || !proto.Equal(projected, pet) {
		t.Fatal("published actual guard snapshot differs from the captured owned Pet", err)
	}
	if before.GetCoin() != after.GetCoin() || before.GetDiamond() != after.GetDiamond() || before.GetPoint() != after.GetPoint() || !p6CopySeriesJSONEqual(t, items.Items, current.Items) || after.GetPoint() != 0 {
		t.Fatal("historical roster imported ordinary inventory or wallet")
	}
	if err = db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=1").Update("enabled", false) })
	return f
}

func p7PartyActualEnd(t *testing.T, f *p1Fixture, start *pb.SCRushFightStart) *pb.CSRushFightEnd {
	t.Helper()
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7GenericMechanismRushInputAt(series, 30.1)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := f.s.ComputeP6GenericMechanismBattle(f.ctx, input, nil)
	if err != nil || out.Outcome.Authoritative {
		t.Fatal("independent test-client source computation", err)
	}
	result := int32(0)
	if out.Outcome.Winner == 0 {
		result = 1
	}
	return &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: out.OriginalOperates}
}

func TestP7PartyRushBusinessRealStarsAdditionContinuationColdConcurrentOnce(t *testing.T) {
	f := p7PartyBusinessFixture(t)
	p7StarActivityOperations75(t, f)
	p7SectionPublish(t)
	p7PublishRushAddition(t)
	p7PublishContinuation(t)
	var last *pb.CSRushFightEnd
	var accepted *pb.SCRushFightEnd
	baselineAtt := float64(0)
	for floor := int32(1); floor <= 4; floor++ {
		if floor == 4 {
			options, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			option := options[0].(*pb.SCRushGetSectionAddition)
			if option.GetCostStar() != 3 {
				t.Fatal("three actual wins did not earn independent spendable stars")
			}
			if _, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAddition{Select: option.StarResult3, Star: proto.Int32(3)}, 20710, time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		query, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
		if err != nil || len(query[0].(*pb.SCRushCurrentInfo).TaskInfoList) != 3 {
			p7PartyScopeFailure(t, f)
			t.Fatal("normal complete party original Ready entry", floor, err)
		}
		task := query[0].(*pb.SCRushCurrentInfo).TaskInfoList[0]
		for _, candidate := range query[0].(*pb.SCRushCurrentInfo).TaskInfoList {
			if candidate.GetStar() == 1 {
				task = candidate
			}
		}
		request := &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: task.TaskId, Star: proto.Int32(1)}
		start, err := f.s.p7Start(f.ctx, f.id, request, 20700+int64(floor), false)
		if err != nil || len(start.GetBattlePlayer().GetFormation()) != 2 {
			t.Fatal("actual party Start", floor, err)
		}
		if floor == 3 {
			baselineAtt = start.BattlePlayer.Formation[0].Prop.GetAtt()
		}
		if floor == 4 && start.BattlePlayer.Formation[0].Prop.GetAtt() != math.Floor(baselineAtt*1.03) {
			t.Fatal("earned addition did not affect actual next party battle")
		}
		var row db.P7RushManualCalculation
		if err = db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil || row.DriverPolicy != constUtil.P7GenericMechanismDriverSource || row.ScopeProof != constUtil.P7GenericMechanismCompatibilityProof || row.Status != constUtil.P7RushManualPending || len(row.Terminal) != 0 {
			t.Fatal("new party pending source/proof", err)
		}
		end := p7PartyActualEnd(t, f, start)
		if end.GetResult() != 1 {
			t.Fatal("actual source lost; preserve source and never replace result", floor)
		}
		if floor == 1 {
			before, game := f.profile(t)
			for _, variant := range []string{"winner", "hp", "transform", "duplicate"} {
				bad := proto.Clone(end).(*pb.CSRushFightEnd)
				switch variant {
				case "winner":
					bad.Result = proto.Int32(0)
				case "hp":
					bad.OptRecord.AttackerTotalLeftHp = proto.Float64(bad.OptRecord.GetAttackerTotalLeftHp() + 1)
				case "transform":
					bad.OptRecord.AttackerTransOpt = &pb.BattleTransOperate{}
				case "duplicate":
					bad.OptRecord.SkillOpts = append(bad.OptRecord.SkillOpts, proto.Clone(bad.OptRecord.SkillOpts[0]).(*pb.BattleSkillOperate))
				}
				if _, err = f.s.p7End(f.ctx, f.id, bad); err == nil {
					t.Fatal("forged whole End accepted", variant)
				}
				after, afterGame := f.profile(t)
				if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, game, afterGame) {
					t.Fatal("rejected End changed ledger/wallet", variant)
				}
			}
		}
		if floor == 4 {
			// 冻结permission在关闭准入后履约，冷重登不重算/更换来源。
			if err = db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			restored, e := f.s.p7Start(f.ctx, f.id, request, 20700+int64(floor), false)
			if e != nil || !proto.Equal(restored, start) {
				t.Fatal("cold pending start/source changed", e)
			}
		}
		workers := 1
		if f.mysql.Database != "" {
			workers = 4
		}
		type answer struct {
			messages []proto.Message
			err      error
		}
		answers := make(chan answer, workers)
		for i := 0; i < workers; i++ {
			go func() { messages, e := f.s.p7End(f.ctx, f.id, end); answers <- answer{messages: messages, err: e} }()
		}
		for i := 0; i < workers; i++ {
			result := <-answers
			if result.err != nil {
				t.Fatal("same End transaction", result.err)
			}
			body := result.messages[0].(*pb.SCRushFightEnd)
			if i > 0 && !proto.Equal(body, accepted) {
				t.Fatal("concurrent End receipt differs")
			}
			accepted = body
		}
		last = end
	}
	before, game := f.profile(t)
	if before.GetPoint() != 40 || p1Count(t, &db.P7RushManualCalculation{}, "player_id=? AND status=?", f.id, constUtil.P7RushManualAccepted) != 4 || p1Count(t, &db.P6BattleRecord{}, "player_id=? AND authoritative=?", f.id, true) != 4 {
		t.Fatal("real party wins did not use exactly four accepted ledger rows")
	}
	f.restart(t)
	repeated, err := f.s.p7End(f.ctx, f.id, last)
	if err != nil || !proto.Equal(repeated[0], accepted) {
		t.Fatal("cold accepted End receipt", err)
	}
	after, current := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, game, current) {
		t.Fatal("cold duplicate minted resources")
	}
	// 实际星活动与本批胜利支付原point商店，未预写任何支付资源。
	for _, initialize := range []func(context.Context) error{f.s.InitializeShop, f.s.InitializeP4} {
		if err = initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	p7PointShopOperations75(t, f)
	var activity db.PeriodInstance
	if err = db.GetIns().Where("kind=? AND enabled=?", constUtil.P7RushStarEvent, true).First(&activity).Error; err != nil {
		t.Fatal(err)
	}
	list, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(activity.ID)})
	if err != nil {
		t.Fatal(err)
	}
	view := list[0].(*pb.SCActivityInfo)
	if len(view.ItemList) != 1 || view.ItemList[0].GetStatus() != 1 {
		t.Fatal("earned party stars did not unlock original activity")
	}
	if _, err = f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(activity.ID), Index: proto.Int32(1), GetResources: view.ItemList[0].RewardList}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("p4Point_1"), BuyCount: proto.Int32(1)}, 20720, time.Now()); err != nil {
		t.Fatal(err)
	}
	funded, goods := f.profile(t)
	if funded.GetPoint() != 40 || simpleOwned(funded, goods, "item_021") != 1 || funded.GetCoin() != before.GetCoin() || funded.GetDiamond() != before.GetDiamond() {
		t.Fatal("real party activity and point-shop cost or material differs")
	}
}

// 失败时只输出实际投影的小型字段，不输出表集或改变资格来掩盖错误。
func p7PartyScopeFailure(t *testing.T, f *p1Fixture) {
	t.Helper()
	p, g := f.profile(t)
	if err := f.s.PrepareP5GuildPrayerProperties(f.ctx, db.GetIns(), p, g, time.Now()); err != nil {
		t.Log("Ready source prayer projection:", err)
		return
	}
	party, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Log("Ready actual battlePlayer projection:", err)
		return
	}
	raw, _ := json.Marshal(party)
	t.Logf("Ready actual battlePlayer SHA=%s fields=%s", p6Digest(raw), raw)
	var cfg db.P7RushStageRule
	if err = db.GetIns().Where("kind=1 AND floor=1 AND star=1 AND enabled=?", true).First(&cfg).Error; err != nil {
		t.Log("Ready actual stage:", err)
		return
	}
	monster, err := f.s.validateP7Stage(cfg)
	if err != nil {
		t.Log("Ready actual monster:", err)
		return
	}
	start := &pb.SCRushFightStart{FightId: proto.String("ready-failure-diagnostic"), RandSeed: proto.Int64(0), BattlePlayer: party, BattleMonsterList: []*pb.BattleMonster{monster}, RushType: proto.Int32(1), FightScene: proto.String(cfg.Scene), MaxRound: proto.Int32(cfg.MaxRound), WinKey: proto.Int32(cfg.WinKey), WinValue: proto.Int32(cfg.WinValue)}
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err == nil {
		err = validateP7PartyRushScope(series)
	}
	t.Log("Ready frozen actual party scope:", err)
}

// 显式原HTTP Start供测试客户端独立计算End，无DB访问、不设置业务权威。
func TestP7PartyRushExportOriginalHTTPStart(t *testing.T) {
	file := os.Getenv("HYJS_P7_PARTY_HTTP_START")
	if file == "" {
		t.Skip("explicit original HTTP Start export not requested")
	}
	output := os.Getenv("HYJS_P7_PARTY_HTTP_OUTPUT")
	root, err := filepath.Abs("../../analysis/p7")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{file, output} {
		absolute, e := filepath.Abs(name)
		if e != nil || !strings.HasPrefix(absolute, root+string(os.PathSeparator)) {
			t.Fatal("owned P7 source/output required")
		}
	}
	raw, err := readTestAnalysis75(file)
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_PARTY_HTTP_START_SHA256") {
		t.Fatal("original HTTP Start SHA", err)
	}
	start := &pb.SCRushFightStart{}
	if proto.Unmarshal(raw, start) != nil {
		t.Fatal("original typed Start malformed")
	}
	autoAt, err := strconv.ParseFloat(os.Getenv("HYJS_P7_PARTY_HTTP_AUTO_AT"), 64)
	if err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7PartyRushInputAt(series, autoAt)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil || out.Outcome.Authoritative || validateP7PartyRushDiagnosticTerminal(input, out) != nil {
		t.Fatal("actual independent typed9 computation", err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write(encoded); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}
