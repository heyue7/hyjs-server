package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// This opt-in exporter traverses actual isolated P7 profile/diagnostic producer
// and persisted P6 gzip input. Its finite level22 stage is explicitly local;
// no original private-server floor, natural paid source or winner is invented.
func TestP6ExportActualP7RushDiagnosticSources(t *testing.T) {
	folder := os.Getenv("HYJS_P6_RUSH_SOURCE_OUTPUT")
	if folder == "" {
		t.Skip("explicit isolated source export disabled")
	}
	absolute, err := filepath.Abs(folder)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(analysis, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatal("source evidence must stay under analysis/p6")
	}
	if err = os.MkdirAll(absolute, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(absolute, "manifest.json")); err == nil {
		t.Fatal("preserve existing immutable source")
	}
	cases := []map[string]any{}
	for _, kind := range []int32{constUtil.P7RushTrial, constUtil.P7RushElite} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			f := p7Setup(t)
			cfg := p7DiagnosticStage(t, f)
			if kind == constUtil.P7RushElite {
				cfg.Kind, cfg.Star, cfg.Floor = kind, 0, 0
				if err = db.GetIns().Save(&cfg).Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err = f.s.formationList(f.ctx, f.id); err != nil {
				t.Fatal(err)
			}
			before, gameBefore := f.profile(t)
			start, prepareErr := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(kind), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(cfg.Star)}, 310)
			if prepareErr != nil {
				t.Fatal(prepareErr)
			}
			var row db.P6BattleRecord
			if err = db.GetIns().First(&row, "player_id=? AND source=? AND source_fight=?", f.id, constUtil.P7RushReplaySource, start.GetFightId()).Error; err != nil {
				t.Fatal(err)
			}
			actual, decodeErr := decodeP6Record(&row)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if start.BattlePlayer == nil || start.BattlePlayer.Inst == nil || len(start.BattleMonsterList) != 1 || start.BattleMonsterList[0].Inst == nil {
				t.Fatal("original typed constructor requires actual player/monster Inst messages")
			}
			series, freezeErr := f.s.FreezeP6RushSeriesInput(start, kind == constUtil.P7RushTrial, true, map[string][]string{"scope": {cfg.Source}}, actual.ClientVersion)
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			out, computeErr := f.s.ComputeP6RushSeries(f.ctx, series, nil)
			if computeErr != nil || out.Authoritative || row.Authoritative {
				t.Fatal("diagnostic source calculation", computeErr)
			}
			if !proto.Equal(series.Template.Attacker, actual.Attacker) || !proto.Equal(series.Template.Monster, actual.Monster) || series.Template.Seed != actual.Seed || series.Template.BattleType != actual.BattleType {
				t.Fatal("typed original source differs from actual persisted P7 producer")
			}
			// The SC projection is only a diagnostic transport for the exact source.
			// Explicit1/1 preserves original typed Clear defaults; nil would become0.
			wire := &pb.SCFightPlayback{Fightid: proto.String(start.GetFightId()), RandSeed: proto.Int64(start.GetRandSeed()), BattleType: proto.Int32(actual.BattleType), Attacker: proto.Clone(start.BattlePlayer).(*pb.BattlePlayer), Defender2: proto.Clone(start.BattleMonsterList[0]).(*pb.BattleMonster), MaxRound: proto.Int32(series.Template.MaxRound), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
			index := len(cases)
			name := fmt.Sprintf("%03d", index)
			write := func(suffix string, value any) string {
				var raw []byte
				var e error
				if msg, ok := value.(proto.Message); ok {
					raw, e = proto.MarshalOptions{Deterministic: true}.Marshal(msg)
				} else {
					raw, e = json.MarshalIndent(value, "", "  ")
				}
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(absolute, name+suffix), raw, 0644); e != nil {
					t.Fatal(e)
				}
				return p6Digest(raw)
			}
			typedSHA := write(".SCRushFightStart.pb", start)
			wireSHA := write(".pb", wire)
			inputSHA := write(".frozen-input.json", actual)
			seriesSHA := write(".rush-series.json", series)
			outSHA := write(".go-outcome.json", out)
			f.restart(t)
			again, coldErr := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(kind), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(cfg.Star)}, 310)
			if coldErr != nil || !proto.Equal(start, again) {
				t.Fatal("cold actual typed source drift", coldErr)
			}
			after, gameAfter := f.profile(t)
			if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, gameBefore, gameAfter) {
				t.Fatal("diagnostic source mutated real isolated profile resources")
			}
			cases = append(cases, map[string]any{"case": fmt.Sprintf("actual-local-p7-rush-kind%d-original-type%d", kind, actual.BattleType), "wire": name + ".pb", "wireSHA256": wireSHA, "seed": actual.Seed, "battleType": actual.BattleType, "firstCamp": 0, "fightId": start.GetFightId(), "typedCallback": name + ".SCRushFightStart.pb", "typedCallbackSHA256": typedSHA, "frozenInput": name + ".frozen-input.json", "frozenInputSHA256": inputSHA, "rushSeries": name + ".rush-series.json", "rushSeriesSHA256": seriesSHA, "goOutcome": name + ".go-outcome.json", "goOutcomeSHA256": outSHA, "source": cfg.Source, "playerAutomatic": true, "originalTypedDefaultPlayerAutomatic": false, "originalAttackerFactor": 1, "originalDefenderFactor": 1, "nativeScope": "original-pve-automatic-ai", "status": "awaiting-original-automatic-ai"})
		})
	}
	if len(cases) != 2 {
		t.Fatal("two original callback modes required")
	}
	manifest := map[string]any{"scope": "Actual isolated P7 diagnostic producer/persisted P6 input/cold source at finite level22; originalHero047 actors. Original server floors and natural paid source unknown. Explicit automaticPlayer choice plus originalClear1/1. No expected native winner.", "engineVersion": constUtil.P6EngineVersion, "nativeCompared": false, "authoritative": false, "cases": cases}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(absolute, "manifest.json"), append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
