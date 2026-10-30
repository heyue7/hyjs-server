package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func training85Rows(t *testing.T) []db.WarriorGrowthRule {
	t.Helper()
	var rows []db.WarriorGrowthRule
	if err := db.GetIns().Where("id IN ?", []string{"training_1", "training_2", "training_3", "breach"}).Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func training85RestoreLegacy(t *testing.T) {
	t.Helper()
	for i, rule := range training85Rules() {
		source := rule.v1GoSource
		if i == 1 {
			source = rule.v1SQLSource
		}
		if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", rule.id).Updates(map[string]any{
			"enabled": true, "distribution": rule.v1Distribution, "critical_multiplier": 1, "reset_seconds": 0, "source": source,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func training85RestoreDefaults(t *testing.T, f *p1Fixture) {
	t.Helper()
	if err := f.s.InitializeWarriorGrowth(f.ctx); err != nil {
		t.Error(err)
		return
	}
	for _, rule := range training85Defaults() {
		if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", rule.ID).Updates(map[string]any{
			"enabled": rule.Enabled, "distribution": rule.Distribution, "critical_multiplier": rule.CriticalMultiplier,
			"reset_seconds": rule.ResetSeconds, "source": rule.Source,
		}).Error; err != nil {
			t.Error(err)
		}
	}
}

func TestWarriorTraining85MigrationAtomicAndIdempotent(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeWarriorGrowth(f.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { training85RestoreDefaults(t, f) })
	training85RestoreLegacy(t)
	before := training85Rows(t)
	if len(before) != 4 || before[0].ID != "breach" {
		t.Fatal("expected all four rules")
	}
	if err := f.s.ApplyWarriorTraining85(f.ctx); err != nil {
		t.Fatal("known Go/SQL legacy rules", err)
	}
	after := training85Rows(t)
	if after[0] != before[0] || !reflect.DeepEqual(after[1:], training85Defaults()) {
		t.Fatal("migration did not preserve breach or install exact v3", after)
	}
	if err := f.s.ApplyWarriorTraining85(f.ctx); err != nil || !reflect.DeepEqual(training85Rows(t), after) {
		t.Fatal("v3 replay changed rules", err)
	}
	for _, sqlSource := range []bool{false, true} {
		training85RestoreLegacy(t)
		for _, rule := range training85Rules() {
			source := rule.v1GoSource
			if sqlSource {
				source = rule.v1SQLSource
			}
			if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", rule.id).Update("source", source).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := f.s.ApplyWarriorTraining85(f.ctx); err != nil || !reflect.DeepEqual(training85Rows(t), after) {
			t.Fatal("complete Go/SQL legacy source", sqlSource, err)
		}
	}
	training85RestoreLegacy(t)
	frozen := training85Rows(t)
	injected := errors.New("test second update failure")
	writes := 0
	callback := db.GetIns().Callback().Update()
	if err := callback.Before("gorm:update").Register("training85:test:failure", func(tx *gorm.DB) {
		if tx.Statement.Table == (db.WarriorGrowthRule{}).TableName() {
			writes++
			if writes == 2 {
				tx.AddError(injected)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := f.s.ApplyWarriorTraining85(f.ctx)
	if removeErr := callback.Remove("training85:test:failure"); removeErr != nil {
		t.Fatal(removeErr)
	}
	if !errors.Is(err, injected) || writes != 2 || !reflect.DeepEqual(training85Rows(t), frozen) {
		t.Fatal("partial update was not rolled back", writes, err)
	}
	for _, mutation := range []struct {
		name   string
		values map[string]any
	}{
		{"source", map[string]any{"source": "admin modified"}},
		{"distribution", map[string]any{"distribution": "[[1,100]]"}},
		{"disabled", map[string]any{"enabled": false}},
		{"multiplier", map[string]any{"critical_multiplier": 2}},
		{"reset", map[string]any{"reset_seconds": 1}},
		{"partial_v3", map[string]any{"source": constUtil.WarriorTraining85Source2, "distribution": constUtil.WarriorTraining85Distribution2}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			training85RestoreLegacy(t)
			if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", "training_2").Updates(mutation.values).Error; err != nil {
				t.Fatal(err)
			}
			frozen := training85Rows(t)
			if err := f.s.ApplyWarriorTraining85(f.ctx); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
				t.Fatal("drift accepted", err)
			}
			if !reflect.DeepEqual(training85Rows(t), frozen) {
				t.Fatal("rejected migration changed a rule")
			}
		})
	}
	training85RestoreLegacy(t)
	if err := db.GetIns().Delete(&db.WarriorGrowthRule{}, "id = ?", "training_2").Error; err != nil {
		t.Fatal(err)
	}
	missing := training85Rows(t)
	if err := f.s.ApplyWarriorTraining85(f.ctx); err == nil || !reflect.DeepEqual(training85Rows(t), missing) {
		t.Fatal("missing row caused a partial migration", err)
	}
	// Restore the isolated fixture's defaults for any later tests using this DB.
	if err := f.s.InitializeWarriorGrowth(f.ctx); err != nil {
		t.Fatal(err)
	}
	training85RestoreLegacy(t)
	if err := f.s.ApplyWarriorTraining85(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestWarriorTraining85DistributionAndBoundaries(t *testing.T) {
	for _, invalid := range []string{"[]", "[[1,0]]", "[[10001,1]]", "[[-10001,1]]", "[[1,9223372036854775807],[2,1]]", "bad"} {
		if _, err := training85ParseDistribution(invalid); err == nil {
			t.Fatal("invalid rule accepted", invalid)
		}
	}
	for _, tc := range []struct {
		current, cap int32
		want         int64
	}{{0, 10000, 9500}, {2500, 10000, 9063}, {5000, 10000, 7750}, {7500, 10000, 5563}, {9000, 10000, 3830}, {9900, 10000, 2639}, {10000, 10000, 2500}, {0, 2147483647, 9500}, {1073741823, 2147483647, 7750}, {2147483647, 2147483647, 2500}} {
		got, err := training85GradientThreshold(tc.current, tc.cap)
		if err != nil || got != tc.want {
			t.Fatalf("threshold %d/%d got %d want %d: %v", tc.current, tc.cap, got, tc.want, err)
		}
	}
	last := int64(10001)
	for current := int32(0); current <= 10000; current++ {
		threshold, err := training85GradientThreshold(current, 10000)
		if err != nil || threshold > last {
			t.Fatal("positive threshold did not decrease with current ratio", current, threshold, last, err)
		}
		last = threshold
	}
	for _, state := range []struct{ current, cap int32 }{{23, 445}, {220, 720}, {879, 16636}, {5000, 10000}, {9900, 10000}} {
		threshold, err := training85GradientThreshold(state.current, state.cap)
		if err != nil {
			t.Fatal(err)
		}
		for _, dice := range []int64{0, threshold - 1, threshold, 9999} {
			var previous int64
			for _, rate := range []int64{60, 90, 140} {
				calls := 0
				draw := func(int64) (int64, error) {
					calls++
					if calls == 1 {
						return dice, nil
					}
					return 0, nil
				}
				change, err := training85GradientChange(state.current, state.cap, rate, draw)
				if err != nil || (dice < threshold && change <= 0) || (dice >= threshold && change >= 0) {
					t.Fatal("same state and sign draw changed sign across tiers", state, rate, dice, change, err)
				}
				magnitude := change
				if magnitude < 0 {
					magnitude = -magnitude
				}
				if magnitude < previous {
					t.Fatal("tier magnitude reversed", state, rate, dice, change, previous)
				}
				previous = magnitude
			}
		}
	}
	positive := func(int64) (int64, error) { return 0, nil }
	negative := func(bound int64) (int64, error) {
		if bound == constUtil.WarriorTraining85GradientScale {
			return bound - 1, nil
		}
		return 0, nil
	}
	if value, err := training85GradientChange(0, 445, 140, negative); err != nil || value != 0 {
		t.Fatal("zero cannot fall further", value, err)
	}
	if value, err := training85GradientChange(445, 445, 140, positive); err != nil || value != 0 {
		t.Fatal("full attribute cannot grow further", value, err)
	}
	if value, err := training85GradientChange(445, 445, 140, negative); err != nil || value >= 0 {
		t.Fatal("full attribute still has negative outcomes", value, err)
	}
	for _, tc := range []struct {
		bps  int64
		want int64
	}{{140, 6}, {-140, -6}, {1, 1}, {-1, -1}, {0, 0}} {
		if got := training85LegacyChange(445, tc.bps); got != tc.want {
			t.Fatal("legacy rounding or sign changed", tc, got)
		}
	}
	if !training85UsesGradient(3, constUtil.WarriorTraining85Source3) || training85UsesGradient(3, constUtil.WarriorTraining85V1GoSource3) || training85UsesGradient(2, constUtil.WarriorTraining85Source3) || training85UsesGradient(3, "operator rule") {
		t.Fatal("v3 source dispatch")
	}
}

var training85ThresholdBenchmarkResult int64

func BenchmarkWarriorTraining85GradientThreshold(b *testing.B) {
	states := [][2]int32{{879, 16636}, {220, 720}, {23, 445}, {19, 367}, {1073741823, 2147483647}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state := states[i%len(states)]
		result, err := training85GradientThreshold(state[0], state[1])
		if err != nil {
			b.Fatal(err)
		}
		training85ThresholdBenchmarkResult = result
	}
}

func training85ProtocolSetup(t *testing.T) *p1Fixture {
	t.Helper()
	f := p1Setup(t)
	f.s.InstallGrowth74Resources()
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	// Initialize the save hooks in the production order. p1Setup already owns
	// the isolated P1 schema and its fixture-only protected first profile.
	for _, initialize := range []func(context.Context) error{
		f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeWarriorGrowth,
		f.s.InitializeOpenServerActivity, f.s.InitializePlayerOpeningCalendar, f.s.InitializeDailyTasks,
		f.s.InitializePayments, f.s.InitializeP4, f.s.InitializeP5,
	} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	// A separate unprotected role exercises the personal opening calendar as
	// well as the live training and confirmation save paths.
	account, err := LoginAccount(f.ctx, "training85-"+t.Name(), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if account.Id <= 1 {
		t.Fatal("training fixture requires an unprotected role")
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "训练协议验证", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	if !f.s.P1Ready || !f.s.P4Ready || !f.s.P5Ready {
		t.Fatal("training fixture save hooks are not ready")
	}
	var calendar db.PlayerOpeningCalendar
	if err = db.GetIns().First(&calendar, "player_id=?", f.id).Error; err != nil || calendar.StartAt <= 0 {
		t.Fatal("training role personal opening calendar", err)
	}
	activityCtx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	opening, err := loadPlayerOpenServerRule(db.GetIns().WithContext(activityCtx), f.id)
	now := LocalPlayNow(activityCtx).UnixMilli()
	if err != nil || opening == nil || !opening.Enabled || now < opening.StartAt || now >= opening.EndAt {
		t.Fatal("training fixture requires a live personal opening period", err)
	}
	training85RestoreDefaults(t, f)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "coin,1000000;diamond,1000;item_007,1000")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// Exercises the original request/receipt and persisted profile transaction. The
// finite level and budget are fixture prerequisites, not natural acquisition.
func TestWarriorTraining85ProtocolCostsPreviewConfirmCancelRetryCold(t *testing.T) {
	f := training85ProtocolSetup(t)
	t.Cleanup(func() { training85RestoreDefaults(t, f) })
	p, _ := f.profile(t)
	guid := p.GetHeroWarId()
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	request := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(1)}
	beforeP, beforeG := f.profile(t)
	beforeRaw, _ := json.Marshal(beforeG)
	if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", "training_3").Update("distribution", "[[80,0]]").Error; err != nil {
		t.Fatal(err)
	}
	draws := 0
	draw := func(int64) (int64, error) { draws++; return 0, nil }
	if _, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, 84999, draw); err == nil || draws != 0 {
		t.Fatal("invalid rule consumed randomness or was accepted", err)
	}
	if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", "training_3").Update("distribution", "[[141,1]]").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, 84999, draw); err == nil || draws != 0 {
		t.Fatal("v3 source with drifted distribution was accepted", err)
	}
	if err := db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", "training_3").Update("distribution", constUtil.WarriorTraining85Distribution3).Error; err != nil {
		t.Fatal(err)
	}
	failed := errors.New("test random failure")
	if _, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, 84999, func(int64) (int64, error) { return 0, failed }); !errors.Is(err, failed) {
		t.Fatal("random failure was not returned", err)
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("validation or random failure partially committed")
	}
	stamp := int64(85000)
	for costType := int32(1); costType <= 3; costType++ {
		for numType, count := range []int32{1, 5, 10} {
			name := fmt.Sprintf("cost%d-count%d", costType, count)
			t.Run(name, func(t *testing.T) {
				stamp += 10
				beforeP, beforeG := f.profile(t)
				beforeW, err := p5Warrior(beforeG, guid)
				if err != nil {
					t.Fatal(err)
				}
				beforeValues := growthValues(beforeW.GetTraining())
				request := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(costType), NumType: proto.Int32(int32(numType + 1))}
				draws := 0
				draw := func(bound int64) (int64, error) {
					draws++
					if bound == constUtil.WarriorTraining85GradientScale {
						return 0, nil
					}
					return bound - 1, nil
				}
				out, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, stamp, draw)
				if err != nil {
					t.Fatal("real training transaction", err)
				}
				response := out[0].(*pb.SCWarriorTraining)
				paidP, paidG := f.profile(t)
				paidW, err := p5Warrior(paidG, guid)
				if err != nil || !proto.Equal(paidW, response.GetGuid()) || growthValues(paidW.GetTraining()) != beforeValues || !trainingPending(paidW.GetTempTraining()) {
					t.Fatal("preview was not persisted separately", err)
				}
				want := map[string]int64{"item_007": int64([]int32{5, 4, 3}[costType-1] * count)}
				if costType == 2 {
					want["coin"] = int64(3000 * count)
				} else if costType == 3 {
					want["diamond"] = int64(5 * count)
				}
				if len(response.GetCost()) != len(want) {
					t.Fatal("cost shape", response.GetCost())
				}
				for _, cost := range response.GetCost() {
					if want[cost.GetId()] != int64(cost.GetNum()) || simpleOwned(beforeP, beforeG, cost.GetId())-simpleOwned(paidP, paidG, cost.GetId()) != int64(cost.GetNum()) {
						t.Fatal("original cost mismatch", cost)
					}
				}
				delta := growthValues(paidW.GetTempTraining())
				if delta == [4]int32{} || draws < int(count)*4 {
					t.Fatal("missing independent attribute draws", delta, draws)
				}
				used := draws
				retry, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, stamp, draw)
				if err != nil || !proto.Equal(retry[0], out[0]) || draws != used {
					t.Fatal("same-stamp replay sampled or spent twice", err)
				}
				if _, err = f.s.warriorTraining85WithDraw(ctx(request), f.id, request, stamp+1, draw); err == nil || draws != used {
					t.Fatal("pending preview accepted another training", err)
				}
				if (costType == 1 && numType == 0) || (costType == 3 && numType == 2) {
					f.restart(t)
				}
				cold, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, stamp, draw)
				if err != nil || !proto.Equal(cold[0], out[0]) || draws != used {
					t.Fatal("cold preview receipt", err)
				}
				result := int32((int(costType) + numType) % 2)
				replace := &pb.CSWarriorTrainingReplace{Guid: proto.String(guid), Result: proto.Int32(result)}
				confirmed, err := f.s.warriorTrainingReplace(ctx(replace), f.id, replace, stamp+1)
				if err != nil {
					t.Fatal("confirm or cancel", err)
				}
				finalP, finalG := f.profile(t)
				finalW, err := p5Warrior(finalG, guid)
				if err != nil || trainingPending(finalW.GetTempTraining()) {
					t.Fatal("pending result remained", err)
				}
				values := growthValues(finalW.GetTraining())
				for i := range values {
					expected := beforeValues[i]
					if result == 1 {
						expected += delta[i]
					}
					if values[i] != expected {
						t.Fatal("confirmed/cancelled value", i, values, expected)
					}
				}
				for id := range want {
					if simpleOwned(finalP, finalG, id) != simpleOwned(paidP, paidG, id) {
						t.Fatal("confirmation or cancellation changed cost", id)
					}
				}
				replaced, err := f.s.warriorTrainingReplace(ctx(replace), f.id, replace, stamp+1)
				if err != nil || !proto.Equal(replaced[0], confirmed[0]) {
					t.Fatal("replace replay", err)
				}
				if _, err = f.s.warriorTraining85WithDraw(ctx(request), f.id, request, stamp, draw); err == nil {
					t.Fatal("stale preview receipt overwrote confirmed state")
				}
			})
		}
	}
	// Exercise the public production path with the cryptographic source as well.
	request = &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(1)}
	if out, err := f.s.warriorTraining(ctx(request), f.id, request, stamp+10); err != nil || !trainingPending(out[0].(*pb.SCWarriorTraining).GetGuid().GetTempTraining()) {
		t.Fatal("production cryptographic training", err)
	}
}

// Each pair starts at the same per-attribute value and uses the same sign draw.
func TestWarriorTraining85ProtocolSameStateSignAndTierMagnitude(t *testing.T) {
	f := training85ProtocolSetup(t)
	t.Cleanup(func() { training85RestoreDefaults(t, f) })
	var guids []string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.WarriorBagSize = proto.Int32(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,6")
		if err != nil {
			return err
		}
		for _, warrior := range gain.GetWars() {
			w, err := p5Warrior(g, warrior.GetGuid())
			if err != nil {
				return err
			}
			limits, err := f.s.trainingLimits(p, w)
			if err != nil {
				return err
			}
			for i := range limits {
				limits[i] /= 2
			}
			w.Training = growthTraining(limits, 0)
			guids = append(guids, w.GetGuid())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(guids) != 6 {
		t.Fatal("six identical finite fixture warriors required", len(guids))
	}
	for sign := 0; sign < 2; sign++ {
		var previous [4]int32
		for tier := int32(1); tier <= 3; tier++ {
			guid := guids[sign*3+int(tier-1)]
			request := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(tier), NumType: proto.Int32(1)}
			ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
			draw := func(bound int64) (int64, error) {
				if bound == constUtil.WarriorTraining85GradientScale && sign == 1 {
					return bound - 1, nil
				}
				return 0, nil
			}
			out, err := f.s.warriorTraining85WithDraw(ctx, f.id, request, 860000+int64(sign*3)+int64(tier), draw)
			if err != nil {
				t.Fatal("same-state original request", sign, tier, err)
			}
			values := growthValues(out[0].(*pb.SCWarriorTraining).GetGuid().GetTempTraining())
			for i, value := range values {
				if (sign == 0 && value <= 0) || (sign == 1 && value >= 0) {
					t.Fatal("same random sign produced different direction", sign, tier, values)
				}
				if value < 0 {
					value = -value
				}
				if value < previous[i] {
					t.Fatal("higher tier reduced the same-sign magnitude", sign, tier, values, previous)
				}
				previous[i] = value
			}
			if sign == 1 && tier == 1 {
				paidP, paidG := f.profile(t)
				paidW, err := p5Warrior(paidG, guid)
				if err != nil {
					t.Fatal(err)
				}
				original := growthValues(paidW.GetTraining())
				cancel := &pb.CSWarriorTrainingReplace{Guid: proto.String(guid), Result: proto.Int32(0)}
				cancelCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, cancel)
				if _, err := f.s.warriorTrainingReplace(cancelCtx, f.id, cancel, 870001); err != nil {
					t.Fatal("cancel negative preview", err)
				}
				finalP, finalG := f.profile(t)
				finalW, err := p5Warrior(finalG, guid)
				if err != nil || growthValues(finalW.GetTraining()) != original || simpleOwned(finalP, finalG, "item_007") != simpleOwned(paidP, paidG, "item_007") {
					t.Fatal("cancel changed attributes or refunded cost", err)
				}
			}
		}
	}
}

func TestWarriorTraining85ProtocolBoundariesAndAdminCompatibility(t *testing.T) {
	f := training85ProtocolSetup(t)
	t.Cleanup(func() { training85RestoreDefaults(t, f) })
	var near, zero, custom string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,3")
		if err != nil {
			return err
		}
		near, zero, custom = gain.GetWars()[0].GetGuid(), gain.GetWars()[1].GetGuid(), gain.GetWars()[2].GetGuid()
		w, err := p5Warrior(g, near)
		if err != nil {
			return err
		}
		limits, err := f.s.trainingLimits(p, w)
		if err != nil {
			return err
		}
		for i := range limits {
			limits[i]--
		}
		w.Training = growthTraining(limits, 0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	low := func(bound int64) (int64, error) {
		if bound == constUtil.WarriorTraining85GradientScale {
			return bound - 1, nil
		}
		return 0, nil
	}
	high := func(bound int64) (int64, error) {
		if bound == constUtil.WarriorTraining85GradientScale {
			return 0, nil
		}
		return bound - 1, nil
	}
	negative := &pb.CSWarriorTraining{Guid: proto.String(zero), CostType: proto.Int32(1), NumType: proto.Int32(3)}
	zeroOut, err := f.s.warriorTraining85WithDraw(ctx(negative), f.id, negative, 850001, low)
	if err != nil || trainingPending(zeroOut[0].(*pb.SCWarriorTraining).GetGuid().GetTempTraining()) {
		t.Fatal("lower bound or empty preview", err)
	}
	positive := &pb.CSWarriorTraining{Guid: proto.String(near), CostType: proto.Int32(3), NumType: proto.Int32(3)}
	nearOut, err := f.s.warriorTraining85WithDraw(ctx(positive), f.id, positive, 850002, high)
	if err != nil || growthValues(nearOut[0].(*pb.SCWarriorTraining).GetGuid().GetTempTraining()) != [4]int32{1, 1, 1, 1} {
		t.Fatal("upper bound should clip to the remaining one point", err)
	}
	confirm := &pb.CSWarriorTrainingReplace{Guid: proto.String(near), Result: proto.Int32(1)}
	if _, err = f.s.warriorTrainingReplace(ctx(confirm), f.id, confirm, 850003); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := f.profile(t)
	if _, err = f.s.warriorTraining85WithDraw(ctx(positive), f.id, positive, 850004, high); err == nil {
		t.Fatal("all-maximum training charged again")
	}
	afterP, afterG := f.profile(t)
	if simpleOwned(beforeP, beforeG, "item_007") != simpleOwned(afterP, afterG, "item_007") || beforeP.GetDiamond() != afterP.GetDiamond() {
		t.Fatal("maximum rejection spent resources")
	}
	// A custom rule keeps the original floor/minimum semantics with v3 present.
	if err = db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id = ?", "training_3").Updates(map[string]any{"source": "operator custom", "distribution": "[[140,100]]"}).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSWarriorTraining{Guid: proto.String(custom), CostType: proto.Int32(3), NumType: proto.Int32(1)}
	customOut, err := f.s.warriorTraining85WithDraw(ctx(request), f.id, request, 850005, high)
	if err != nil {
		t.Fatal(err)
	}
	customW := customOut[0].(*pb.SCWarriorTraining).GetGuid()
	limits, err := f.s.trainingLimits(afterP, customW)
	if err != nil {
		t.Fatal(err)
	}
	got := growthValues(customW.GetTempTraining())
	for i, limit := range limits {
		want := max(int32(1), int32(int64(limit)*140/10000))
		if got[i] != want {
			t.Fatal("custom source received v3 gradient", got, limits)
		}
	}
}
