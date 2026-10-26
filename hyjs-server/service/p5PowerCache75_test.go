package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The finite level/material/gear inputs are explicit. There is no guild, prayer,
// injected fight result or direct mutation of the original formation consumers.
func p5PowerCacheFixture75(t *testing.T) (*p1Fixture, string, string) {
	t.Helper()
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var hero, gear string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(70)
		p.Coin = proto.Int64(1000000)
		p.Stamina = proto.Int32(200)
		p.Energy = proto.Int32(200)
		gain, err := f.s.GrantFixedReward(p, g, "hero_003,1;equip_033,1;item_005,180")
		if err != nil {
			return err
		}
		hero = gain.Wars[0].GetGuid()
		gear = gain.Equips[0].GetGuid()
		w, err := p5Warrior(g, hero)
		if err == nil {
			w.Level = proto.Int32(70)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	on := &pb.CSFormationOn{Id: proto.String("hero_003"), Guid: proto.String(hero), FormatPos: proto.Int32(1)}
	if _, err := f.s.p5FormationOnNative(p5PowerCtx75(f, on), f.id, on, 755001); err != nil {
		t.Fatal(err)
	}
	equip := &pb.CSFormationOn{Id: proto.String("equip_033"), Guid: proto.String(gear), FormatPos: proto.Int32(1)}
	if _, err := f.s.p5EquipmentFormationOn(p5PowerCtx75(f, equip), f.id, equip, 755004); err != nil {
		t.Fatal(err)
	}
	// Wearing itself is an existing consumer; initial stored cache is required
	// correct so a later failure specifically identifies a growth missing refresh.
	p5PowerEqual75(t, f, "original wearing")
	return f, hero, gear
}
func p5PowerCtx75(f *p1Fixture, b proto.Message) context.Context {
	return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
}
func p5PowerEqual75(t *testing.T, f *p1Fixture, label string) int32 {
	t.Helper()
	p, g := f.profile(t)
	want, err := f.s.formationFightPower(p, g)
	if err != nil || p.GetFightPower() != want {
		t.Fatalf("%s persisted power=%d actual=%d err=%v", label, p.GetFightPower(), want, err)
	}
	return want
}
func p5PowerState75(t *testing.T, f *p1Fixture) string {
	t.Helper()
	p, g := f.profile(t)
	b, err := json.Marshal([]any{p, g})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func p5PowerPlayerQuery75(t *testing.T, f *p1Fixture) *pb.Player {
	t.Helper()
	request := &pb.CSPlayer{}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	out, err := f.s.dispatch(p5PowerCtx75(f, request), &model.GamePacket{PlayerId: f.id, ZoneId: int32(f.s.OriginalServerID()), Session: row.Session, Body: request}, "isolated-power75")
	if err != nil {
		t.Fatal("original CSPlayer", err)
	}
	for _, v := range out {
		if x, ok := v.(*pb.SCPlayer); ok {
			return x.Player
		}
	}
	t.Fatal("CSPlayer missing original reply")
	return nil
}

func TestP5PowerCache75OrdinaryQuotedEquipmentGrowthAndReplay(t *testing.T) {
	f, _, guid := p5PowerCacheFixture75(t)
	before, _ := f.profile(t)
	prior := before.GetFightPower()
	_, g := f.profile(t)
	gear, err := f.s.equipmentRecord(g, guid)
	if err != nil {
		t.Fatal(err)
	}
	quality := f.s.table("Equip", "id", gear.GetEquipId())["quality"]
	var coin int64
	for _, r := range f.s.Tables["EquipExp"] {
		if r["quality"] == quality && r["level"] == strconv.Itoa(int(gear.GetLevel())) {
			coin, err = strconv.ParseInt(r["coin"], 10, 32)
			break
		}
	}
	if err != nil || coin <= 0 {
		t.Fatal("original quote", coin, err)
	}
	req := &pb.CSEquipStrengthen{Guid: proto.String(guid), Times: proto.Int32(1), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(coin))}}}
	out, err := f.s.equipmentStrengthenStamped(p5PowerCtx75(f, req), f.id, req, 755002)
	if err != nil {
		t.Fatal(err)
	}
	actual := p5PowerEqual75(t, f, "ordinary quoted strength")
	if actual <= prior {
		t.Fatal("real equipped growth did not increase power")
	}
	paid, _ := f.profile(t)
	if before.GetCoin()-paid.GetCoin() != coin {
		t.Fatal("original quote charge")
	}
	saved := p5PowerState75(t, f)
	again, err := f.s.equipmentStrengthenStamped(p5PowerCtx75(f, req), f.id, req, 755002)
	if err != nil || !proto.Equal(out[0], again[0]) || saved != p5PowerState75(t, f) {
		t.Fatal("same stamp charged or grew twice", err)
	}
	if got := p5PowerPlayerQuery75(t, f); got.GetFightPower() != actual {
		t.Fatal("CSPlayer stale after ordinary strength", got.GetFightPower(), actual)
	}
}
func TestP5PowerCache75OriginalTalentGrowthAndReplay(t *testing.T) {
	f, guid, _ := p5PowerCacheFixture75(t)
	before, bg := f.profile(t)
	prior := before.GetFightPower()
	old := simpleOwned(before, bg, "item_005")
	req := &pb.CSWarriorTalent{Guid: proto.String(guid), Talent: proto.String("1"), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(10000)}, {Id: proto.String("item_005"), Num: proto.Int32(30)}}}
	out, err := f.s.p5WarriorTalent(p5PowerCtx75(f, req), f.id, req, 755003)
	if err != nil {
		t.Fatal(err)
	}
	actual := p5PowerEqual75(t, f, "original talent")
	var growthPlayer *pb.Player
	for _, frame := range out {
		if view, ok := frame.(*pb.SCPlayer); ok {
			growthPlayer = view.Player
		}
	}
	if growthPlayer == nil || growthPlayer.GetFightPower() != actual {
		t.Fatal("same talent transaction returned stale SCPlayer power", growthPlayer, actual)
	}
	if actual <= prior {
		t.Fatal("actual talent did not increase power")
	}
	paid, pg := f.profile(t)
	if before.GetCoin()-paid.GetCoin() != 10000 || old-simpleOwned(paid, pg, "item_005") != 30 {
		t.Fatal("original talent charge")
	}
	saved := p5PowerState75(t, f)
	again, err := f.s.p5WarriorTalent(p5PowerCtx75(f, req), f.id, req, 755003)
	if err != nil || !proto.Equal(out[0], again[0]) || saved != p5PowerState75(t, f) {
		t.Fatal("talent same stamp charged twice", err)
	}
	if p5PowerPlayerQuery75(t, f).GetFightPower() != actual {
		t.Fatal("CSPlayer stale after talent")
	}
}
func TestP5PowerCache75ColdCSPlayerRepairsOnlyStalePower(t *testing.T) {
	f, _, _ := p5PowerCacheFixture75(t)
	before, g := f.profile(t)
	want, err := f.s.formationFightPower(before, g)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit corrupt-cache prerequisite, with all actual actors/gear unchanged.
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.FightPower = proto.Int32(1); return nil }); err != nil {
		t.Fatal(err)
	}
	broken, bg := f.profile(t)
	brokenGame, _ := json.Marshal(bg)
	f.restart(t)
	got := p5PowerPlayerQuery75(t, f)
	if got.GetFightPower() != want {
		t.Fatalf("cold CSPlayer kept old power=%d actual=%d", got.GetFightPower(), want)
	}
	after, ag := f.profile(t)
	if after.GetFightPower() != want {
		t.Fatal("repaired reply not persisted")
	}
	compare := proto.Clone(after).(*pb.Player)
	compare.FightPower = broken.FightPower
	agraw, _ := json.Marshal(ag)
	if !proto.Equal(compare, broken) || string(agraw) != string(brokenGame) {
		t.Fatal("CSPlayer repair changed other assets/state")
	}
}

func TestP5PowerCache75HotMovementCurrencyAndUnslottedPetSkipRecalculation(t *testing.T) {
	f, _, _ := p5PowerCacheFixture75(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.FightPower = proto.Int32(1)
		p.PetBagSize = proto.Int32(20)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A poisoned power-only factor makes any accidental eager recomputation fail.
	// Valid actor, skill, equipment and currency tables remain intact.
	factor := f.s.table("GameConfigure", "StaticName", "fightpower_att_factor")
	original := factor["NameValue"]
	factor["NameValue"] = "invalid-power-only-probe"
	defer func() { factor["NameValue"] = original }()
	read := func(body proto.Message) {
		t.Helper()
		p, _, err := f.s.resourceProfile(p5PowerCtx75(f, body), f.id)
		if err != nil || p.GetFightPower() != 1 {
			t.Fatalf("hot %T ran power calculator or repaired unrelated cache: %v", body, err)
		}
	}
	read(&pb.CSGetPaymentInfo{})
	read(&pb.CSPetList{})
	move := &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(8), Y: proto.Float32(0), Z: proto.Float32(9)}}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.dispatch(p5PowerCtx75(f, move), &model.GamePacket{PlayerId: f.id, ZoneId: int32(f.s.OriginalServerID()), Session: row.Session, Body: move}, "isolated-hot-power75"); err != nil {
		t.Fatal("ordinary movement recalculated", err)
	}
	moved, mg := f.profile(t)
	if moved.GetFightPower() != 1 || mg.Position.GetCurrent().GetX() != 8 {
		t.Fatal("ordinary move not actually committed or stale cache rewritten")
	}
	first := &pb.CSPetFirstRecruit{}
	out, err := f.s.p9PetFirst(p5PowerCtx75(f, first), f.id, 755100, first)
	if err != nil {
		t.Fatal("unmounted first pet must not enter formula", err)
	}
	reply := out[0].(*pb.SCPetFirstRecruit)
	if len(reply.GetGet().GetPets()) != 1 {
		t.Fatal("actual original first pet not granted")
	}
	added, ag := f.profile(t)
	if added.GetFightPower() != 1 {
		t.Fatal("unmounted pet repaired unrelated power cache")
	}
	for _, slot := range ag.Formation {
		for _, thing := range slot.Things {
			if thing.GetGuid() == reply.GetGet().Pets[0].GetGuid() {
				t.Fatal("first pet probe unexpectedly mounted")
			}
		}
	}
	saved := p5PowerState75(t, f)
	again, err := f.s.p9PetFirst(p5PowerCtx75(f, first), f.id, 755100, first)
	if err != nil || !proto.Equal(again[0], out[0]) || saved != p5PowerState75(t, f) {
		t.Fatal("first pet lost callback changed assets", err)
	}
	// Explicit CSPlayer must enter the formula, unlike all earlier hot operations.
	if _, _, err = f.s.resourceProfile(p5PowerCtx75(f, &pb.CSPlayer{}), f.id); err == nil {
		t.Fatal("CSPlayer bypassed poisoned power formula")
	}
	if p5PowerState75(t, f) != saved {
		t.Fatal("failed explicit power repair partially saved")
	}
	factor["NameValue"] = original
	expected, err := f.s.formationFightPower(added, ag)
	if err != nil {
		t.Fatal(err)
	}
	got := p5PowerPlayerQuery75(t, f)
	if got.GetFightPower() != expected {
		t.Fatal("restored explicit CSPlayer did not repair exact power", got.GetFightPower(), expected)
	}
}
