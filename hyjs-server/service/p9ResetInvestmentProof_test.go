package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p9ProofFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := legacyResetFixture(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetRecycle, f.s.InitializeP9PetIntimacy, f.s.InitializeP9CopyIntimacy} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P9PetsReady = true
	if err := db.GetIns().Model(&db.P9PetMarketRule{}).Where("id=?", constUtil.P9MarketRuleID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return f
}

// Persist an old or externally changed instance without invoking any current
// growth operation, so the fixture cannot accidentally establish a proof.
func p9ProofLegacySave(tx *gorm.DB, id int64, g *model.GameState, pet *pb.Pet) error {
	data, err := proto.Marshal(pet)
	if err != nil {
		return err
	}
	if err := tx.Model(&db.P9PetInstance{}).Where("player_id=? AND guid=?", id, pet.GetGuid()).Update("data", data).Error; err != nil {
		return err
	}
	return p9ProjectPet(g, pet)
}

func p9AssertRejected(t *testing.T, f *p1Fixture, pet *pb.Pet, operation func() error, reason string) {
	t.Helper()
	beforeP, beforeG := f.profile(t)
	before, _ := json.Marshal(beforeG)
	beforePet, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	if err = operation(); err == nil || err.Error() != reason {
		t.Fatal("unexpected result for unproven pet", err)
	}
	afterP, afterG := f.profile(t)
	after, _ := json.Marshal(afterG)
	afterPet, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
	if err != nil || !proto.Equal(beforeP, afterP) || string(before) != string(after) || !proto.Equal(beforePet, afterPet) {
		t.Fatal("rejected operation changed pet or resources", err)
	}
}

func TestP9ResetAndDecomposeProtectUnprovenLegacyInvestments(t *testing.T) {
	f := p9ProofFixture(t)
	var intimate, grown *pb.Pet
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(75), proto.Int64(1000000), proto.Int64(200)
		if _, err := f.s.GrantFixedReward(p, g, "item_124,10"); err != nil {
			return err
		}
		pets, err := f.s.P9GrantPet(tx, p, "pet_002", 2)
		if err != nil {
			return err
		}
		intimate, grown = pets[0], pets[1]
		intimate.Eat.Exp = proto.Int32(1)
		grown.Level = proto.Int32(2)
		if err = p9ProofLegacySave(tx, f.id, g, intimate); err != nil {
			return err
		}
		return p9ProofLegacySave(tx, f.id, g, grown)
	}); err != nil {
		t.Fatal(err)
	}
	for index, pet := range []*pb.Pet{intimate, grown} {
		if index == 1 {
			level := &pb.CSPetLevelup{Guid: grown.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}
			if _, err := f.s.p9PetLevelup(f.ctx, f.id, level, 996002); err != nil {
				t.Fatal("tracked growth after legacy progress", err)
			}
			var investment db.P9PetInvestment
			if err := db.GetIns().First(&investment, "guid=? AND player_id=?", grown.GetGuid(), f.id).Error; err != nil {
				t.Fatal(err)
			}
			if _, proven := p9InvestmentProof(investment.Source); proven {
				t.Fatal("old unknown investment was marked complete")
			}
		}
		stamp := int64(996010 + index*10)
		p9AssertRejected(t, f, pet, func() error {
			_, err := f.s.p9PetReset(f.ctx, f.id, &pb.CSPetReset{Guid: pet.Guid}, stamp)
			return err
		}, constUtil.P9PetResetInvestmentMissing)
		p9AssertRejected(t, f, pet, func() error {
			_, err := f.s.p9PetDecompose(f.ctx, f.id, &pb.CSPetDecompose{Guids: []string{pet.GetGuid()}}, stamp+1)
			return err
		}, constUtil.P9PetResetInvestmentMissing)
	}
	f.restart(t)
	p9AssertRejected(t, f, grown, func() error {
		_, err := f.s.p9PetReset(f.ctx, f.id, &pb.CSPetReset{Guid: grown.Guid}, 996020)
		return err
	}, constUtil.P9PetResetInvestmentMissing)
}

func TestP9ProofFreePaidColdAndUnexpectedChange(t *testing.T) {
	f := p9ProofFixture(t)
	var pet, interaction, tampered *pb.Pet
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(75), proto.Int64(1000000), proto.Int64(300)
		if _, err := f.s.GrantFixedReward(p, g, "item_124,10"); err != nil {
			return err
		}
		pets, err := f.s.P9GrantPet(tx, p, "pet_002", 3)
		if err == nil {
			pet, interaction, tampered = pets[0], pets[1], pets[2]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	bad := &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1000)}}}
	if _, err := f.s.p9PetLevelup(f.ctx, f.id, bad, 997000); err == nil {
		t.Fatal("insufficient resources accepted")
	}
	var premature int64
	if err := db.GetIns().Model(&db.P9PetInvestment{}).Where("guid=?", pet.GetGuid()).Count(&premature).Error; err != nil || premature != 0 {
		t.Fatal("failed growth persisted a proof", premature, err)
	}
	on := &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}
	if _, err := f.s.p9GuardOn(f.ctx, f.id, on, 997001); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, err := f.s.ApplyP9CopyIntimacy(tx, f.id, p, g, "sweep:proof-fixture")
		return err
	}); err != nil {
		t.Fatal("real free copy intimacy", err)
	}
	off := &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}
	if _, _, err := f.s.DispatchP9PetNext(f.ctx, f.id, off, 997002); err != nil {
		t.Fatal(err)
	}
	level := &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}
	if _, err := f.s.p9PetLevelup(f.ctx, f.id, level, 997003); err != nil {
		t.Fatal("paid growth after proven free intimacy", err)
	}
	var investment db.P9PetInvestment
	if err := db.GetIns().First(&investment, "guid=? AND player_id=?", pet.GetGuid(), f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := p9InvestmentProof(investment.Source); !ok {
		t.Fatal("free and paid growth lost complete proof")
	}
	reset := &pb.CSPetReset{Guid: pet.Guid}
	result, err := f.s.p9PetReset(f.ctx, f.id, reset, 997004)
	if err != nil {
		t.Fatal("proven free and paid reset", err)
	}
	response := result[0].(*pb.SCPetReset)
	if len(response.GetGet().GetPets()) != 1 || response.GetGet().GetPets()[0].GetGuid() == pet.GetGuid() {
		t.Fatal("native reset replacement")
	}
	f.restart(t)
	again, err := f.s.p9PetReset(f.ctx, f.id, reset, 997004)
	if err != nil || !proto.Equal(response, again[0]) {
		t.Fatal("cold proven reset replay", err)
	}
	shown, err := f.s.p9PetHudongShow(f.ctx, f.id, &pb.CSPetHudongShow{Guid: interaction.Guid})
	if err != nil {
		t.Fatal(err)
	}
	favorite := shown[0].(*pb.SCPetHudongShow).GetPet().GetEat().GetCritTrend()
	if _, err = f.s.p9PetHudongAt(f.ctx, f.id, &pb.CSPetHudong{Guid: interaction.Guid, Type: proto.Int32(favorite)}, 997005, time.Now()); err != nil {
		t.Fatal("original free interaction", err)
	}
	if _, err = f.s.p9PetReset(f.ctx, f.id, &pb.CSPetReset{Guid: interaction.Guid}, 997006); err != nil {
		t.Fatal("proven free interaction reset", err)
	}
	level.Guid = tampered.Guid
	if _, err = f.s.p9PetLevelup(f.ctx, f.id, level, 997007); err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		owned, err := p9OwnedPet(tx, f.id, tampered.GetGuid())
		if err != nil {
			return err
		}
		owned.Exp = proto.Int64(owned.GetExp() + 1)
		return p9ProofLegacySave(tx, f.id, g, owned)
	}); err != nil {
		t.Fatal(err)
	}
	p9AssertRejected(t, f, tampered, func() error {
		_, err := f.s.p9PetReset(f.ctx, f.id, &pb.CSPetReset{Guid: tampered.Guid}, 997008)
		return err
	}, constUtil.P9PetResetInvestmentInvalid)
	if _, err = f.s.p9PetLevelup(f.ctx, f.id, level, 997009); err != nil {
		t.Fatal("continued growth after invalid proof", err)
	}
	investment = db.P9PetInvestment{}
	if err = db.GetIns().First(&investment, "guid=?", tampered.GetGuid()).Error; err != nil || !strings.Contains(investment.Source, constUtil.P9PetInvestmentInvalidProof) {
		t.Fatal("changed state did not permanently invalidate proof", err)
	}
	p9AssertRejected(t, f, tampered, func() error {
		_, err := f.s.p9PetReset(f.ctx, f.id, &pb.CSPetReset{Guid: tampered.Guid}, 997010)
		return err
	}, constUtil.P9PetResetInvestmentMissing)
}

func TestP9ProofInvalidatedByActualAdminPetEdit(t *testing.T) {
	f := p9ProofFixture(t)
	for _, init := range []func(context.Context) error{func(ctx context.Context) error { return AdminInitialize(ctx, "") }, f.s.InitializeGlobalGameAccounts} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "proof-" + uuid.NewString(), PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	var pet *pb.Pet
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(75), proto.Int64(1000000), proto.Int64(100)
		if _, err := f.s.GrantFixedReward(p, g, "item_124,2"); err != nil {
			return err
		}
		pets, err := f.s.P9GrantPet(tx, p, "pet_002", 1)
		if err == nil {
			pet = pets[0]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	level := &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}
	if _, err := f.s.p9PetLevelup(f.ctx, f.id, level, 998001); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	current, err := f.s.AdminPlayerInventory(ctx, f.id, "", "", nil, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	req := model.AdminPlayerInventoryRequest{Version: current.Version, RequestID: uuid.NewString(), Operation: "update", Category: "pet", Key: pet.GetGuid(), Attributes: map[string]int64{"level": 3}, Reason: "isolated proof protection verification"}
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, req); err != nil {
		t.Fatal("actual admin edit", err)
	}
	var investment db.P9PetInvestment
	if err := db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil || !strings.Contains(investment.Source, constUtil.P9PetInvestmentInvalidProof) {
		t.Fatal("admin edit retained a complete proof", err)
	}
	p9AssertRejected(t, f, pet, func() error {
		_, err := f.s.p9PetReset(f.ctx, f.id, &pb.CSPetReset{Guid: pet.Guid}, 998002)
		return err
	}, constUtil.P9PetResetInvestmentMissing)
}
