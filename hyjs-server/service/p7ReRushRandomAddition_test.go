package service

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP7ReRushRandomAdditionSamplesTenWithoutDuplicates(t *testing.T) {
	source := rand.New(rand.NewSource(20261101))
	seen := map[int32]map[string]bool{3: {}, 6: {}, 9: {}}
	for i := 0; i < 1000; i++ {
		options, err := p7ReRushGenerateOptions(source)
		if err != nil || p7ReRushValidateOptions(options) != nil {
			t.Fatalf("random options: %+v %v", options, err)
		}
		for _, cost := range []int32{3, 6, 9} {
			key, err := p7ReRushAdditionKey(options.selected(cost), cost)
			if err != nil {
				t.Fatal(err)
			}
			seen[cost][key] = true
		}
	}
	for cost, keys := range seen {
		if len(keys) != 10 {
			t.Fatalf("cost %d did not cover all ten types: %v", cost, keys)
		}
	}
	bad := p7ReRushAdditionOptions{Version: constUtil.P7ReRushRandomAddition, Three: "peratt,0.03", Six: "peratt,0.06", Nine: "hit,0.09"}
	if p7ReRushValidateOptions(bad) == nil {
		t.Fatal("duplicate attribute accepted")
	}
}

func TestP7ReRushRandomAdditionFreezesAndAppliesTenTypes(t *testing.T) {
	f := p7Setup(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	player, game := f.profile(t)
	party, err := f.s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	power, err := f.s.formationFightPowerFromParty(party)
	if err != nil || power <= 0 || len(party.Formation) == 0 {
		t.Fatalf("formation: power=%d err=%v", power, err)
	}
	for floor := int32(1); floor <= 3; floor++ {
		if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", fmt.Sprintf("re-rush:%d:3", floor)).Update("recommended_power", float64(power)).Error; err != nil {
			t.Fatal(err)
		}
	}
	call := func(req proto.Message, stamp int64) proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP7(f.ctx, f.id, req, stamp)
		if err != nil || !handled || len(out) == 0 {
			t.Fatalf("%T: %v", req, err)
		}
		return out[0]
	}
	sweep := call(&pb.CSRush3Hit{}, 120001).(*pb.SCRush3Hit)
	if sweep.GetIsValid() != 1 || len(sweep.Floor) != 3 {
		t.Fatalf("missing completed section: %v", sweep)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var section db.P7ReRushSection
	if err = db.GetIns().Where("player_id=? AND run_id=? AND section=1", f.id, state.RunID).First(&section).Error; err != nil || !section.Claimed {
		t.Fatalf("earned section: %v", err)
	}
	keys := []string{"perhp", "peratt", "perpdef", "permdef", "hit", "dodge", "cri", "antiCri", "damageAdd", "damageReduce"}
	propValue := func(prop *pb.BaseProp, key string, wire bool) float64 {
		switch key {
		case "perhp":
			if wire {
				return prop.GetPerhp()
			}
			return prop.GetHp()
		case "peratt":
			if wire {
				return prop.GetPeratt()
			}
			return prop.GetAtt()
		case "perpdef":
			if wire {
				return prop.GetPerpdef()
			}
			return prop.GetPdef()
		case "permdef":
			if wire {
				return prop.GetPermdef()
			}
			return prop.GetMdef()
		case "hit":
			return prop.GetHit()
		case "dodge":
			return prop.GetDodge()
		case "cri":
			return prop.GetCri()
		case "antiCri":
			return prop.GetAntiCri()
		case "damageAdd":
			return prop.GetDamageAdd()
		case "damageReduce":
			return prop.GetDamageReduce()
		}
		t.Fatalf("unknown test property %s", key)
		return 0
	}
	// 用已经真实完成的区段证明逐项核对十种属性和三个星级的战斗、界面数值。
	for _, key := range keys {
		for _, cost := range []int32{3, 6, 9} {
			var others []string
			for _, candidate := range keys {
				if candidate != key {
					others = append(others, candidate)
				}
			}
			chosen := map[int32]string{3: others[0], 6: others[1], 9: key}
			if cost != 9 {
				chosen[cost], chosen[9] = key, chosen[cost]
			}
			options := p7ReRushAdditionOptions{Version: constUtil.P7ReRushRandomAddition, Three: chosen[3] + ",0.03", Six: chosen[6] + ",0.06", Nine: chosen[9] + ",0.09"}
			encoded, err := json.Marshal(options)
			if err != nil || p7ReRushValidateOptions(options) != nil {
				t.Fatal("isolated options", err)
			}
			if err = db.GetIns().Model(&section).Updates(map[string]any{"options_json": string(encoded), "selected": options.selected(cost), "spent_stars": cost}).Error; err != nil {
				t.Fatal(err)
			}
			bonus, err := p7ReRushBonus(db.GetIns(), state)
			if err != nil {
				t.Fatalf("%s/%d: %v", key, cost, err)
			}
			rate := float64(cost) / 100
			if got := propValue(p7ReRushBonusProjection(bonus), key, true); math.Abs(got-rate) > 1e-9 {
				t.Fatalf("%s/%d UI got=%v", key, cost, got)
			}
			boosted, err := f.s.p7ApplyReRushBonus(db.GetIns(), state, party)
			if err != nil {
				t.Fatalf("%s/%d battle: %v", key, cost, err)
			}
			before := propValue(party.Formation[0].Prop, key, false)
			want := before + float64(cost)
			if strings.HasPrefix(key, "per") {
				want = math.RoundToEven(before * (1 + rate))
			}
			if got := propValue(boosted.Formation[0].Prop, key, false); math.Abs(got-want) > 1e-9 {
				t.Fatalf("%s/%d battle got=%v want=%v", key, cost, got, want)
			}
		}
	}
	if err = db.GetIns().Model(&section).Updates(map[string]any{
		"options_json": `{"version":"random-v1","three":"peratt,0.03","six":"peratt,0.06","nine":"hit,0.09"}`,
		"selected":     "hit,0.09", "spent_stars": 9,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p7ReRushBonus(db.GetIns(), state); err == nil {
		t.Fatal("duplicate frozen options were accepted")
	}
	if err = db.GetIns().Model(&section).Updates(map[string]any{"options_json": "", "selected": "", "spent_stars": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRushAddition{Star: proto.Int32(3), Select: proto.String("perhp,0.03")}, 120002); err == nil {
		t.Fatal("selection before opening its offer was accepted")
	}
	first := call(&pb.CSRushGetSectionAddition{}, 0).(*pb.SCRushGetSectionAddition)
	f.restart(t)
	again := call(&pb.CSRushGetSectionAddition{}, 0).(*pb.SCRushGetSectionAddition)
	if !proto.Equal(first, again) {
		t.Fatal("reopen or restart changed frozen options")
	}
	chosen := &pb.CSRushAddition{Star: proto.Int32(6), Select: first.StarResult6}
	accepted := call(chosen, 120003)
	f.restart(t)
	if replay := call(chosen, 120003); !proto.Equal(accepted, replay) {
		t.Fatal("cold addition receipt changed")
	}
	if os.Getenv("HYJS_P7_MYSQL_DATABASE") != "" {
		// 隔离 MySQL 中重现旧版表结构，确认迁移不会改写已选择或已打开的旧加成。
		if err = db.GetIns().Model(&section).Updates(map[string]any{
			"options_json": "", "selected": "perpdef,0.09", "spent_stars": 9,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Migrator().DropColumn(&db.P7ReRushSection{}, "OptionsJSON"); err != nil {
			t.Fatal("prepare old schema", err)
		}
		if err = f.s.InitializeP7ReRush(f.ctx); err != nil {
			t.Fatal("upgrade old schema", err)
		}
		var migrated db.P7ReRushSection
		if err = db.GetIns().First(&migrated, "player_id=? AND run_id=? AND section=1", f.id, state.RunID).Error; err != nil ||
			migrated.Selected != "perpdef,0.09" || migrated.SpentStars != 9 || migrated.OptionsJSON != "" || !migrated.Claimed {
			t.Fatalf("old selection changed: %+v %v", migrated, err)
		}
		bonus, err := p7ReRushBonus(db.GetIns(), state)
		if err != nil || bonus.GetPdef() != .09 || bonus.GetMdef() != .09 {
			t.Fatalf("old defense bonus changed: %v %v", bonus, err)
		}
		if err = db.GetIns().Model(&section).Updates(map[string]any{"selected": "", "spent_stars": 0}).Error; err != nil {
			t.Fatal(err)
		}
		legacy := call(&pb.CSRushGetSectionAddition{}, 0).(*pb.SCRushGetSectionAddition)
		if legacy.GetStarResult3() != "perhp,0.03" || legacy.GetStarResult6() != "peratt,0.06" || legacy.GetStarResult9() != "perpdef,0.09" {
			t.Fatal("old open offer changed", legacy)
		}
	}
}
