package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The same finite fixture runs on isolated SQLite or the runner's allocated
// HYJS_P1_MYSQL_DATABASE. It never starts or creates a MySQL service/database.
func recycle61Fixture(t *testing.T, rewards string) *p1Fixture {
	t.Helper()
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking, f.s.InitEquipmentMutations, f.s.InitializeOpenServerActivity} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := db.GetIns().AutoMigrate(&db.P7ExpeditionHero{}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.PetBagSize = proto.Int32(1000)
		p.Coin = proto.Int64(1000000)
		if rewards != "" {
			_, err := f.s.GrantFixedReward(p, g, rewards)
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	return f
}

func recycle61Dispatch(f *p1Fixture, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	return f.s.DispatchP5(ctx, f.id, request, stamp)
}

func recycle61Session(t *testing.T, f *p1Fixture) []byte {
	t.Helper()
	session := []byte("recycle61-test!!")
	// A real session is 16 bytes; keep the fixture independent of login refresh.
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	return session
}

func recycle61Receipt(t *testing.T, f *p1Fixture, operation string, stamp int64, want int64) {
	t.Helper()
	if n := p1Count(t, &db.P5MutationReceipt{}, "player_id=? AND operation=? AND timestamp=?", f.id, operation, stamp); n != want {
		t.Fatalf("receipt count=%d want=%d", n, want)
	}
}

func TestRecycle61EquipmentHundredAtomicAndReceipt(t *testing.T) {
	f := recycle61Fixture(t, "equip_001,100")
	request := equipment40Request(t, f)
	if len(request.Guid) != 100 {
		t.Fatal("finite 100-equipment fixture missing")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		_, err := f.s.GrantFixedReward(p, g, "equip_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tooMany := equipment40Request(t, f)
	before := equipment40Snapshot(t, f)
	if _, _, err := recycle61Dispatch(f, tooMany, 61001); err == nil {
		t.Fatal("101 accepted")
	}
	if before != equipment40Snapshot(t, f) {
		t.Fatal("101 changed profile")
	}
	recycle61Receipt(t, f, "equipment-decompose", 61001, 0)
	initialP, _ := f.profile(t)
	// The final protected object must roll back all earlier tracked refunds.
	if _, err := f.s.p5StrengthenOne(f.ctx, f.id, &pb.CSEquipStrengthenOneKey{Guid: proto.String(request.Guid[0])}, 61002); err != nil {
		t.Fatal(err)
	}
	last := request.Guid[99]
	for _, protection := range []string{"lock", "dispatch", "worn", "missing"} {
		t.Run(protection, func(t *testing.T) {
			originalP, originalG := f.profile(t)
			if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
				switch protection {
				case "lock":
					p5SetKV(g, "p5.lock:"+last, "1")
				case "dispatch":
					p5SetKV(g, "p5.dispatch:"+last, "finite-mission")
				case "worn":
					g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Id: proto.String("equip_001"), Guid: proto.String(last), Pos: proto.Int32(4)})
				case "missing":
					for i, e := range g.Equipment {
						if e.GetGuid() == last {
							g.Equipment = append(g.Equipment[:i], g.Equipment[i+1:]...)
							break
						}
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			snapshot := equipment40Snapshot(t, f)
			if _, _, err := recycle61Dispatch(f, request, 61003); err == nil {
				t.Fatal("final ineligible object accepted")
			}
			if snapshot != equipment40Snapshot(t, f) {
				t.Fatal("failed batch partially refunded/deleted")
			}
			recycle61Receipt(t, f, "equipment-decompose", 61003, 0)
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
				proto.Reset(p)
				proto.Merge(p, originalP)
				*g = *originalG
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
	out, handled, err := recycle61Dispatch(f, request, 61003)
	if err != nil || !handled {
		t.Fatal("100 native equipment failed", err)
	}
	if len(out[0].(*pb.SCEquipDecompose).Cost) != 100 {
		t.Fatal("cost truncated")
	}
	settledP, g := f.profile(t)
	rewards, err := p3Resources(f.s.table("EquipDecompose", "quality", f.s.table("Equip", "id", "equip_001")["quality"])["retRes"])
	if err != nil {
		t.Fatal(err)
	}
	expectedCoin, expectedPoint := initialP.GetCoin(), int64(initialP.GetPoint())
	for _, reward := range rewards {
		switch reward.GetId() {
		case "coin":
			expectedCoin += int64(reward.GetNum()) * 100
		case "point":
			expectedPoint += int64(reward.GetNum()) * 100
		}
	}
	if settledP.GetCoin() != expectedCoin || int64(settledP.GetPoint()) != expectedPoint {
		t.Fatal("tracked refund or 100 original base rewards mismatched")
	}
	if len(g.Equipment) != 1 || p5KV(g, "p5.invest:"+request.Guid[0]) != "" {
		t.Fatal("100 removal or investment settlement mismatch")
	}
	snapshot := equipment40Snapshot(t, f)
	f.restart(t)
	replay, _, err := recycle61Dispatch(f, request, 61003)
	if err != nil || !proto.Equal(out[0], replay[0]) || snapshot != equipment40Snapshot(t, f) {
		t.Fatal("cold exact-body replay changed settlement", err)
	}
	conflict := &pb.CSEquipDecompose{Guid: append([]string(nil), request.Guid...)}
	conflict.Guid[99] = tooMany.Guid[100]
	if _, _, err := recycle61Dispatch(f, conflict, 61003); err == nil {
		t.Fatal("same timestamp different body accepted")
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("conflicting body changed settlement")
	}
	recycle61Receipt(t, f, "equipment-decompose", 61003, 1)
}

func TestRecycle61SelectionHundredReadOnlyAndBounds(t *testing.T) {
	f := recycle61Fixture(t, "equip_001,100")
	session := recycle61Session(t, f)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		_, err := f.s.GrantFixedReward(p, g, "equip_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	for _, n := range []int{-1, 0, 101} {
		if _, err := f.s.RecycleSelection(f.ctx, f.id, session, "equipment", n); err == nil {
			t.Fatalf("invalid limit %d accepted", n)
		}
	}
	if _, err := f.s.RecycleSelection(f.ctx, f.id, session, "invalid", 100); err == nil {
		t.Fatal("invalid kind accepted")
	}
	for _, n := range []int{1, 4, 5, 40, 99, 100} {
		out, err := f.s.RecycleSelection(f.ctx, f.id, session, "equipment", n)
		if err != nil || len(out.GUIDs) != n {
			t.Fatalf("limit=%d selected=%v err=%v", n, out, err)
		}
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("selection changed profile")
	}
	recycle61Receipt(t, f, "equipment-decompose", 61004, 0)
}

func TestRecycle61HeroHundredNativeAtomic(t *testing.T) {
	f := recycle61Fixture(t, "hero_056,100")
	request := &pb.CSWarriorDecompose{}
	p, g := f.profile(t)
	for _, w := range g.Warriors {
		if w.GetGuid() != p.GetHeroWarId() {
			request.Guid = append(request.Guid, w.GetGuid())
		}
	}
	if len(request.Guid) != 100 {
		t.Fatal("finite 100-hero fixture missing")
	}
	session := recycle61Session(t, f)
	selected, err := f.s.RecycleSelection(f.ctx, f.id, session, "hero", 100)
	if err != nil || len(selected.GUIDs) != 100 {
		t.Fatal("100-hero selection", err)
	}
	tooMany := &pb.CSWarriorDecompose{Guid: append(append([]string(nil), request.Guid...), "unowned-101")}
	before101 := equipment40Snapshot(t, f)
	if _, _, err := recycle61Dispatch(f, tooMany, 61005); err == nil || before101 != equipment40Snapshot(t, f) {
		t.Fatal("101 hero boundary", err)
	}

	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { p5SetKV(g, "p5.lock:"+request.Guid[99], "1"); return nil }); err != nil {
		t.Fatal(err)
	}
	before := equipment40Snapshot(t, f)
	if _, _, err := recycle61Dispatch(f, request, 61005); err == nil {
		t.Fatal("final locked hero accepted")
	}
	if before != equipment40Snapshot(t, f) {
		t.Fatal("hero batch partially changed")
	}
	recycle61Receipt(t, f, "warrior-decompose", 61005, 0)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { p5SetKV(g, "p5.lock:"+request.Guid[99], ""); return nil }); err != nil {
		t.Fatal(err)
	}
	_, heldG := f.profile(t)
	held, err := p5Warrior(heldG, request.Guid[99])
	if err != nil {
		t.Fatal(err)
	}
	rawHeld, err := proto.Marshal(held)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P7ExpeditionHero{PlayerID: f.id, GUID: held.GetGuid(), MissionSerial: "finite61-ready-not-claimed", Snapshot: rawHeld}).Error; err != nil {
		t.Fatal(err)
	}
	heldBefore := equipment40Snapshot(t, f)
	if _, _, err := recycle61Dispatch(f, request, 61005); err == nil || heldBefore != equipment40Snapshot(t, f) {
		t.Fatal("held hero bypassed final save guard", err)
	}
	recycle61Receipt(t, f, "warrior-decompose", 61005, 0)
	if err := db.GetIns().Delete(&db.P7ExpeditionHero{}, "player_id=? AND guid=?", f.id, held.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	out, handled, err := recycle61Dispatch(f, request, 61005)
	if err != nil || !handled || len(out[0].(*pb.SCWarriorDecompose).Cost) != 100 {
		t.Fatal("100 hero native transaction", err)
	}
	after := equipment40Snapshot(t, f)
	replay, _, err := recycle61Dispatch(f, request, 61005)
	if err != nil || !proto.Equal(out[0], replay[0]) || after != equipment40Snapshot(t, f) {
		t.Fatal("hero replay changed settlement", err)
	}
}

func TestRecycle61PetHundredProtectionAtomicAndReceipt(t *testing.T) {
	f := recycle61Fixture(t, "")
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetRecycle} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P9PetMarketRule{}).Where("id=?", constUtil.P9MarketRuleID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSPetDecompose{}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, _ *model.GameState) error {
		pets, err := f.s.P9GrantPet(tx, p, "pet_009", 100)
		for _, pet := range pets {
			request.Guids = append(request.Guids, pet.GetGuid())
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var pets []db.P9PetInstance
		if err := db.GetIns().Where("player_id=?", f.id).Order("guid").Find(&pets).Error; err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(pets)
		return equipment40Snapshot(t, f) + string(raw)
	}
	session := recycle61Session(t, f)
	selected, err := f.s.RecycleSelection(f.ctx, f.id, session, "pet", 100)
	if err != nil || len(selected.GUIDs) != 100 {
		t.Fatal("100-pet selection", err)
	}
	last := request.Guids[99]
	for _, protection := range []string{"lock", "dispatch", "guard"} {
		t.Run(protection, func(t *testing.T) {
			_, originalG := f.profile(t)
			if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
				switch protection {
				case "lock":
					p5SetKV(g, "p5.lock:"+last, "1")
				case "dispatch":
					p5SetKV(g, "p5.dispatch:"+last, "finite-mission")
				case "guard":
					g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Id: proto.String("pet_009"), Guid: proto.String(last), Pos: proto.Int32(7)})
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := snapshot()
			if _, err := f.s.p9PetDecompose(f.ctx, f.id, request, 61006); err == nil {
				t.Fatal("final protected pet accepted")
			}
			if before != snapshot() || p1Count(t, &db.P9PetRetired{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P9PetReceipt{}, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P9DecomposeOperation, 61006) != 0 {
				t.Fatal("failed pet batch changed instances, wallet or receipt")
			}
			if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { *g = *originalG; return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
	tooMany := &pb.CSPetDecompose{Guids: append(append([]string(nil), request.Guids...), "unowned-101")}
	before := snapshot()
	if _, err := f.s.p9PetDecompose(f.ctx, f.id, tooMany, 61006); err == nil || before != snapshot() {
		t.Fatal("101 pet boundary changed state", err)
	}
	out, err := f.s.p9PetDecompose(f.ctx, f.id, request, 61006)
	if err != nil || len(out[0].(*pb.SCPetDecompose).Cost) != 100 {
		t.Fatal("100 pet decomposition", err)
	}
	after := snapshot()
	f.restart(t)
	replay, err := f.s.p9PetDecompose(f.ctx, f.id, request, 61006)
	if err != nil || !proto.Equal(out[0], replay[0]) || after != snapshot() {
		t.Fatal("cold pet replay changed settlement", err)
	}
	conflict := &pb.CSPetDecompose{Guids: append([]string(nil), request.Guids...)}
	conflict.Guids[99] = "different-guid"
	if _, err := f.s.p9PetDecompose(f.ctx, f.id, conflict, 61006); err == nil || after != snapshot() {
		t.Fatal("conflicting pet body changed settlement", err)
	}
	if p1Count(t, &db.P9PetRetired{}, "player_id=?", f.id) != 100 {
		t.Fatal("retirement truncated")
	}
}

func TestRecycle61SelectionLargeInventoryTiming(t *testing.T) {
	f := recycle61Fixture(t, "equip_001,100")
	session := recycle61Session(t, f)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		base := g.Equipment[0]
		for i := len(g.Equipment); i < 2000; i++ {
			g.Equipment = append(g.Equipment, proto.Clone(base).(*pb.Equipment))
		}
		for i, e := range g.Equipment {
			e.Guid = proto.String(fmt.Sprintf("large61-%05d", i))
			if i < 1900 {
				p5SetKV(g, "p5.lock:"+e.GetGuid(), "1")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := equipment40Snapshot(t, f)
	started := time.Now()
	out, err := f.s.RecycleSelection(f.ctx, f.id, session, "equipment", 100)
	if err != nil || len(out.GUIDs) != 100 || out.Skipped != 1900 || out.GUIDs[0] != "large61-01900" || out.GUIDs[99] != "large61-01999" {
		t.Fatal("large inventory eligibility, skipped count or stable order", out, err)
	}
	t.Logf("2000 equipment / 1900 locked / 100 eligible candidates: %s; dialect=%s", time.Since(started), db.GetIns().Dialector.Name())
	if before != equipment40Snapshot(t, f) {
		t.Fatal("large selection mutated inventory")
	}
}
