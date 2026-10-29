package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// These are finite progression prerequisites reconstructed from recorded owned
// heroes and gear. Power is always calculated from the resulting original
// panels; no displayed-power override or fabricated battle result is used.
func desertBossCalibrationTeam(t *testing.T, s *GameService, ids []string) (*pb.Player, *model.GameState, int32) {
	t.Helper()
	raw, err := readTestAnalysis75("../../analysis/p6/p5-combined-all76-highest-actors-20261014.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Paths []struct {
			HeroID    string            `json:"heroId"`
			Warrior   *pb.Warrior       `json:"warrior"`
			Equipment []*pb.Equipment   `json:"equipment"`
			Treasures []*pb.Treasure    `json:"treasures"`
			Formation *pb.FormationItem `json:"formation"`
		} `json:"characterPaths"`
	}
	if err = json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	player := &pb.Player{PlayerId: proto.Int64(2), Name: proto.String("沙漠数值隔离夹具"), Level: proto.Int32(100)}
	build := func(progress int32) (*model.GameState, int32) {
		g := &model.GameState{}
		formation, e := s.newTacticalFormation("2", 1)
		if e != nil {
			t.Fatal(e)
		}
		g.TacticalFormations = []*pb.TaticalFormation{formation}
		g.TacticalSkillId = s.tacticalSkill("2", 1)["id"]
		for i, id := range ids {
			found := false
			for _, row := range source.Paths {
				if row.HeroID != id {
					continue
				}
				found = true
				w := proto.Clone(row.Warrior).(*pb.Warrior)
				w.Breached.Level = proto.Int32(1 + 24*progress/10000)
				for _, v := range w.SkillRefine {
					v.Level = proto.Int32(1 + 99*progress/10000)
					row, e := s.specialtyTable("SkillSpecialty", "level", v.GetType(), v.GetLevel())
					if e != nil {
						t.Fatal(e)
					}
					need, e := copySweepNumber(row, "needStrengthLevel")
					if e != nil {
						t.Fatal(e)
					}
					v.StrengthLevel = proto.Int32(int32(need))
				}
				g.Warriors = append(g.Warriors, w)
				for _, original := range row.Equipment {
					v := proto.Clone(original).(*pb.Equipment)
					v.Level = proto.Int32(1 + 259*progress/10000)
					v.RefineLv = proto.Int32(80 * progress / 10000)
					v.Star = proto.Int32(5 * progress / 10000)
					v.ForgeLevel = proto.Int32(15 * progress / 10000)
					g.Equipment = append(g.Equipment, v)
				}
				for _, original := range row.Treasures {
					v := proto.Clone(original).(*pb.Treasure)
					v.StrengthLevel = proto.Int32(1 + 99*progress/10000)
					v.RefineLevel = proto.Int32(30 * progress / 10000)
					g.Treasures = append(g.Treasures, v)
				}
				f := proto.Clone(row.Formation).(*pb.FormationItem)
				f.FormatPos, f.BattlePos = proto.Int32(int32(i+1)), proto.Int32(int32(i+1))
				g.Formation = append(g.Formation, f)
				break
			}
			if !found {
				t.Fatal("missing original actor", id)
			}
		}
		power, e := s.formationFightPower(player, g)
		if e != nil {
			t.Fatal("original formation power", e)
		}
		return g, power
	}
	low, high := int32(0), int32(10000)
	for low < high {
		mid := (low + high) / 2
		_, power := build(mid)
		if power < 500000 {
			low = mid + 1
		} else {
			high = mid
		}
	}
	g, power := build(low)
	if low > 0 {
		prior, previous := build(low - 1)
		if math.Abs(float64(previous)-500000) < math.Abs(float64(power)-500000) {
			g, power = prior, previous
		}
	}
	if math.Abs(float64(power)-500000) > 10000 {
		t.Fatal("formation outside 2 percent of 500k", power)
	}
	player.FightPower = proto.Int32(power)
	return player, g, power
}

func TestGGOBossPhoneCalibrationFixture(t *testing.T) {
	if os.Getenv("HYJS_GGO_BOSS_PHONE_FIXTURE") != "1" {
		t.Skip("explicit owned phone prerequisite only")
	}
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name != "hyjs_test_p1_desert_v4_qa_28b" || os.Getenv("HYJS_P1_MYSQL_HOST") != "127.0.0.1:3306" || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("wrong owned QA allocation")
	}
	f := &p1Fixture{ctx: context.Background(), id: 2, mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}}
	f.open(t)
	t.Cleanup(func() { pool, _ := db.GetIns().DB(); _ = pool.Close() })
	var role db.Profile
	if err := db.GetIns().First(&role, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var account db.GameAccount
	if err := db.AccountsTable(db.GetIns(), "game_accounts").First(&account, role.AccountID).Error; err != nil {
		t.Fatal(err)
	}
	if role.Id <= 1 || account.Protected || !account.Enabled || account.Username != "qa-desert-player-1" {
		t.Fatal("phone role ownership mismatch")
	}
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables, s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{"hero_006", "hero_009", "hero_015", "hero_017", "hero_043"}
	_, source, power := desertBossCalibrationTeam(t, s, ids)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		if g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
			return fmt.Errorf("owned QA role has a pending battle")
		}
		g.Warriors, g.Equipment, g.Treasures, g.Formation = source.Warriors, source.Equipment, source.Treasures, source.Formation
		g.TacticalFormations, g.TacticalSkillId = source.TacticalFormations, source.TacticalSkillId
		p.Level, p.FightPower = proto.Int32(100), proto.Int32(power)
		for _, w := range g.Warriors {
			if w.GetId() == "hero_043" {
				p.HeroWarId = proto.String(w.GetGuid())
			}
		}
		if g.Field != nil && g.Field.Side != nil {
			g.Field.Side.BattlePosHps = nil
			g.Field.Side.CurHp = proto.Float64(1)
			g.Field.Side.Anger = proto.Float64(0)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("only owned QA role2 finite five-hero formation prepared: actual original power %d", power)
}

func TestGGOBossFifteen500kPartyTurnsCalibration(t *testing.T) {
	output := os.Getenv("HYJS_GGO_BOSS_CALIBRATION_OUTPUT")
	if output == "" {
		t.Skip("explicit finite numerical calibration")
	}
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables, s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	teams := [][]string{{"hero_001", "hero_002", "hero_003", "hero_004", "hero_005"}, {"hero_006", "hero_009", "hero_015", "hero_017", "hero_043"}, {"hero_065", "hero_066", "hero_067", "hero_074", "hero_075"}}
	cfg := defaultGGORuntime()
	event := &ggoBossEvent{ID: "ggo-calibration", Boss: cfg.Boss}
	if raw := os.Getenv("HYJS_GGO_BOSS_CALIBRATION_HP"); raw != "" {
		hp, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || hp <= 0 || hp > 1000000000000 {
			t.Fatal("invalid finite calibration HP")
		}
		event.Boss.HP = hp
	}
	var rows []map[string]any
	var total float64
	for teamIndex, ids := range teams {
		p, g, power := desertBossCalibrationTeam(t, s, ids)
		party, err := s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		for member := 0; member < 5; member++ {
			monster, e := s.ggoBossMonster(event, float64(event.Boss.HP))
			if e != nil {
				t.Fatal(e)
			}
			input, e := s.freezeAndQualifyP6GenericMechanisms(t.Context(), model.FrozenBattleInput{Attacker: party, Monster: monster, Seed: int64(280000 + teamIndex*5 + member), MaxRound: int32(event.Boss.PartyRounds * 2), BattleType: constUtil.FieldBossBattleType, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
			if e != nil {
				t.Fatal(e)
			}
			out, _, e := s.ComputeP6GenericMechanismBattle(t.Context(), input, nil)
			if e != nil {
				t.Fatal(e)
			}
			var remaining, playerHP float64
			for _, actor := range out.FinalActors {
				if actor.Camp == 1 {
					remaining += actor.CurrentHP
				} else {
					playerHP += actor.CurrentHP
				}
			}
			damage := float64(event.Boss.HP) - remaining
			total += damage
			rows = append(rows, map[string]any{"team": teamIndex + 1, "member": member + 1, "heroIds": ids, "actualFormationPower": power, "damage": damage, "remainingPlayerHP": playerHP, "finalRound": out.Outcome.FinalRound, "winner": out.Outcome.Winner, "inputSHA256": p6Digest(adminJSON(input))})
		}
	}
	report := map[string]any{"finitePrerequisites": "recorded highest owned heroes and gear, legal progression stages reduced; actual original panel power within 2 percent of 500000; no production account mutation", "scope": "15 independent 10-own-round fights against full-health boss; summed damage estimates shared HP without early-kill clipping, not phone or concurrency evidence", "configuredBoss": event.Boss, "totalDamage": total, "configuredHP": event.Boss.HP, "ratio": total / float64(event.Boss.HP), "rows": rows}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("15 x 500k, ten own rounds: damage %.0f / HP %d = %.4f", total, event.Boss.HP, total/float64(event.Boss.HP))
}
