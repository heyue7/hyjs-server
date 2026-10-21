package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

// This fixture uses a temporary SQLite file and finite original equipment grants.
// It proves the native protocol transaction, without claiming a production role anchor.
func equipment40Fixture(t *testing.T, rewards string) *p1Fixture {
	t.Helper()
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking, f.s.InitEquipmentMutations} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		_, err := f.s.GrantFixedReward(p, g, "coin,1000000;diamond,100;"+rewards)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f
}
func equipment40Snapshot(t *testing.T, f *p1Fixture) string {
	t.Helper()
	p, g := f.profile(t)
	raw, err := json.Marshal([]any{p, g})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func equipment40Request(t *testing.T, f *p1Fixture) *pb.CSEquipDecompose {
	t.Helper()
	_, g := f.profile(t)
	request := &pb.CSEquipDecompose{}
	for _, equip := range g.Equipment {
		request.Guid = append(request.Guid, equip.GetGuid())
	}
	return request
}
func TestEquipmentRecycle40FourBaseOriginalQualityAndCold(t *testing.T) {
	for _, item := range []string{"equip_001", "equip_005"} {
		t.Run(item, func(t *testing.T) {
			f := equipment40Fixture(t, item+",4")
			request := equipment40Request(t, f)
			p, _ := f.profile(t)
			beforeCoin, beforePoint := p.GetCoin(), p.GetPoint()
			row := f.s.table("EquipDecompose", "quality", f.s.table("Equip", "id", item)["quality"])
			original, err := p3Resources(row["retRes"])
			if err != nil {
				t.Fatal(err)
			}
			var coin, point int64
			for _, resource := range original {
				if resource.GetId() == "coin" {
					coin += int64(resource.GetNum()) * 4
				}
				if resource.GetId() == "point" {
					point += int64(resource.GetNum()) * 4
				}
			}
			out, err := f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40001)
			if err != nil {
				t.Fatal(err)
			}
			response := out[0].(*pb.SCEquipDecompose)
			p, g := f.profile(t)
			if len(response.GetCost()) != 4 || len(g.Equipment) != 0 || p.GetCoin() != beforeCoin+coin || int64(p.GetPoint()) != int64(beforePoint)+point {
				t.Fatal("native four-item original returns", response, p)
			}
			snapshot := equipment40Snapshot(t, f)
			f.restart(t)
			replay, err := f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40001)
			if err != nil || !proto.Equal(out[0], replay[0]) || snapshot != equipment40Snapshot(t, f) {
				t.Fatal("cold retry changed original result", err)
			}
		})
	}
}
func TestEquipmentRecycle40TrackedStrengtheningProtectionAndAtomicRetry(t *testing.T) {
	f := equipment40Fixture(t, "equip_001,1;equip_002,1;equip_003,1;equip_004,1")
	request := equipment40Request(t, f)
	_, g := f.profile(t)
	first := g.Equipment[0]
	before, _ := f.profile(t)
	if _, err := f.s.p5StrengthenOne(f.ctx, f.id, &pb.CSEquipStrengthenOneKey{Guid: first.Guid}, 40002); err != nil {
		t.Fatal(err)
	}
	_, g = f.profile(t)
	strengthened, err := f.s.equipmentRecord(g, first.GetGuid())
	if err != nil || strengthened.GetLevel() < 2 || p5KV(g, "p5.invest:"+first.GetGuid()) == "" {
		t.Fatal("real investment missing", err)
	}
	// A later protected selection must roll back the earlier tracked refund.
	protected := request.Guid[3]
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { p5SetKV(g, "p5.lock:"+protected, "1"); return nil }); err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	if _, err = f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40003); err == nil {
		t.Fatal("locked selected equipment accepted")
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("locked batch partially refunded")
	}
	if count := p1Count(t, &db.P5MutationReceipt{}, "player_id=? AND operation=? AND timestamp=?", f.id, "equipment-decompose", 40003); count != 0 {
		t.Fatal("failed request acquired success receipt")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+protected, "")
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Id: proto.String("equip_004"), Guid: proto.String(protected), Pos: proto.Int32(4)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	snapshot = equipment40Snapshot(t, f)
	if _, err = f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40003); err == nil {
		t.Fatal("worn selected equipment accepted")
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("worn batch partially refunded")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		for _, slot := range g.Formation {
			var remain []*pb.FormationThing
			for _, thing := range slot.GetThings() {
				if thing.GetGuid() != protected {
					remain = append(remain, thing)
				}
			}
			slot.Things = remain
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40003)
	if err != nil {
		t.Fatal("same failed timestamp cannot recover", err)
	}
	p, g := f.profile(t)
	if p.GetCoin() != before.GetCoin()+4000 || p.GetPoint() != before.GetPoint()+320 || len(g.Equipment) != 0 || p5KV(g, "p5.invest:"+first.GetGuid()) != "" {
		t.Fatal("tracked actual spend or original base return lost", p)
	}
	snapshot = equipment40Snapshot(t, f)
	f.restart(t)
	replay, err := f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40003)
	if err != nil || !proto.Equal(out[0], replay[0]) || snapshot != equipment40Snapshot(t, f) {
		t.Fatal("tracked cold replay duplicated refund", err)
	}
}
func TestEquipmentRecycle40LegacyProgressRemainsProtected(t *testing.T) {
	f := equipment40Fixture(t, "equip_001,4")
	request := equipment40Request(t, f)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Equipment[3].Level = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	_, err := f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40004)
	if err == nil || !strings.Contains(err.Error(), "培养返还校验失败") || !strings.Contains(err.Error(), request.Guid[3]) {
		t.Fatal("legacy investment should remain protected with selected context", err)
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("untracked final selection lost base gear")
	}
	f.restart(t)
	if _, err = f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40004); err == nil || snapshot != equipment40Snapshot(t, f) {
		t.Fatal("cold retry bypassed investment protection", err)
	}
}

// A real new investment does not make earlier unknown progress refundable.
func TestEquipmentRecycle40TrackedBaselineWithOlderProgressProtected(t *testing.T) {
	f := equipment40Fixture(t, "equip_001,1")
	request := equipment40Request(t, f)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Equipment[0].Level = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p5StrengthenOne(f.ctx, f.id, &pb.CSEquipStrengthenOneKey{Guid: proto.String(request.Guid[0])}, 40005); err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	_, err := f.s.p5EquipmentDecompose(f.ctx, f.id, request, 40006)
	if err == nil || !strings.Contains(err.Error(), "返还后仍有培养进度") {
		t.Fatal("tracked newer spend erased older untracked progress", err)
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("rejected baseline refund escaped transaction")
	}
}

func TestRecycleSelection40EquipmentReadOnlyAndNativeConfirmation(t *testing.T) {
	f := equipment40Fixture(t, "equip_001,4;equip_005,2")
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		for i, e := range g.Equipment {
			e.Guid = proto.String(fmt.Sprintf("equipment40-%03d", i+1))
		}
		p5SetKV(g, "p5.lock:equipment40-001", "1")
		g.Equipment[1].Level = proto.Int32(2)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p5StrengthenOne(f.ctx, f.id, &pb.CSEquipStrengthenOneKey{Guid: proto.String("equipment40-003")}, 40101); err != nil {
		t.Fatal(err)
	}
	session := []byte("equipment40-test")
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	out, err := f.s.RecycleSelection(f.ctx, f.id, session, "equipment", 4)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.GUIDs, ",") != "equipment40-003,equipment40-004,equipment40-005,equipment40-006" || out.Skipped != 2 {
		t.Fatal("original-quality eligibility or stable order", out)
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("read-only selection changed live profile")
	}
	if count := p1Count(t, &db.P5MutationReceipt{}, "player_id=? AND operation=?", f.id, "equipment-decompose"); count != 0 {
		t.Fatal("selection created decomposition receipt")
	}
	if _, err = f.s.RecycleSelection(f.ctx, f.id, []byte("stale-session-40"), "equipment", 4); err == nil {
		t.Fatal("stale session selected materials")
	}
	if _, err = f.s.p5EquipmentDecompose(f.ctx, f.id, &pb.CSEquipDecompose{Guid: out.GUIDs}, 40102); err != nil {
		t.Fatal("selected materials cannot use unchanged native confirm", err)
	}
}

func TestRecycleSelection40HeroRealExpeditionAndLegacyExcluded(t *testing.T) {
	f := equipment40Fixture(t, "hero_056,4;hero_032,1;hero_039,1;hero_exp_001,1")
	if err := db.GetIns().AutoMigrate(&db.P7ExpeditionHero{}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		index := 0
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				continue
			}
			index++
			w.Guid = proto.String(fmt.Sprintf("hero40-%03d", index))
			if index == 2 {
				w.Level = proto.Int32(2)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P7ExpeditionHero{PlayerID: f.id, GUID: "hero40-001", MissionSerial: "finite-hold", Snapshot: []byte{1}}).Error; err != nil {
		t.Fatal(err)
	}
	session := []byte("equipment40-test")
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	out, err := f.s.RecycleSelection(f.ctx, f.id, session, "hero", 4)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.GUIDs, ",") != "hero40-003,hero40-004,hero40-005" {
		t.Fatal("held legacy special or EXP hero selected", out)
	}
	if snapshot != equipment40Snapshot(t, f) {
		t.Fatal("hero selection changed live resources")
	}
}

func TestRecycleSelection40PetOriginalFiltersAndNoMarketWrite(t *testing.T) {
	f := equipment40Fixture(t, "item_001,1")
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
	var wanted []string
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(34)
		pets, err := f.s.P9GrantPet(tx, p, "pet_009", 4)
		if err != nil {
			return err
		}
		blue, err := f.s.P9GrantPet(tx, p, "pet_001", 1)
		if err != nil {
			return err
		}
		if _, err = f.s.P9GrantPet(tx, p, "pet_046", 1); err != nil {
			return err
		}
		pets[0].Level = proto.Int32(2)
		pets[1].Eat.Exp = proto.Int32(1)
		for _, pet := range pets[:2] {
			if err = p9SavePet(tx, f.id, g, pet); err != nil {
				return err
			}
		}
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Id: pets[2].Id, Guid: pets[2].Guid, Pos: proto.Int32(7)})
		wanted = []string{pets[3].GetGuid(), blue[0].GetGuid()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	session := []byte("equipment40-test")
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := equipment40Snapshot(t, f)
	var beforeRows []db.P9PetInstance
	if err := db.GetIns().Where("player_id=?", f.id).Order("guid").Find(&beforeRows).Error; err != nil {
		t.Fatal(err)
	}
	beforeRaw, _ := json.Marshal(beforeRows)
	out, err := f.s.RecycleSelection(f.ctx, f.id, session, "pet", 4)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.GUIDs, ",") != strings.Join(wanted, ",") || out.Skipped != 1 {
		t.Fatal("native pet quality/growth/guard filters", out, wanted)
	}
	var afterRows []db.P9PetInstance
	if err := db.GetIns().Where("player_id=?", f.id).Order("guid").Find(&afterRows).Error; err != nil {
		t.Fatal(err)
	}
	afterRaw, _ := json.Marshal(afterRows)
	if snapshot != equipment40Snapshot(t, f) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("pet trial changed live profile or owned instances")
	}
	if count := p1Count(t, &db.P9PetMarketState{}, "player_id=?", f.id); count != 0 {
		t.Fatal("read-only selection created lazy market state")
	}
}
