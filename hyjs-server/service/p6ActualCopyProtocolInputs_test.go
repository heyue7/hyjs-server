package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func p6ActualCopyPlaybackProjection(start *pb.SCCopyFightStart) *pb.SCFightPlayback {
	// Original Clear sets both factors to1 and SetData(SCCopyFightStart)
	// preserves them. SetPlaybackData reads scalar defaults of0, so carry
	// the original Clear values explicitly in this diagnostic projection.
	return &pb.SCFightPlayback{Fightid: start.FightId, RandSeed: start.RandSeed, BattleType: proto.Int32(1), Attacker: start.BattlePlayer, Defender2: start.BattleMonsterList[0], MaxRound: start.MaxRound, AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
}

func TestP6ActualCopyPlaybackProjectionPreservesOriginalDefaultFactors(t *testing.T) {
	for _, task := range []string{"10", "12"} {
		raw, err := os.ReadFile("../../analysis/p15/actual-copy-starts-20261015/mission3-task" + task + ".SCCopyFightStart.pb")
		if err != nil {
			t.Fatal(err)
		}
		start := &pb.SCCopyFightStart{}
		if err = proto.Unmarshal(raw, start); err != nil {
			t.Fatal(err)
		}
		projection := p6ActualCopyPlaybackProjection(start)
		raw, err = proto.MarshalOptions{Deterministic: true}.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.AttackerFactor == nil || decoded.DefenderFactor == nil || decoded.GetAttackerFactor() != 1 || decoded.GetDefenderFactor() != 1 || !proto.Equal(decoded.Attacker, start.BattlePlayer) || !proto.Equal(decoded.Defender2, start.BattleMonsterList[0]) || decoded.GetRandSeed() != start.GetRandSeed() || decoded.GetMaxRound() != start.GetMaxRound() || decoded.OptRecord != nil {
			t.Fatal("original natural copy parameters were lost through playback encoding")
		}
	}
}

// The opt-in export preserves actual paid source actors/seed/max without running
// calculations, assigning winners or treating canonical source objects as socket bytes.
func TestP6ExportActualPaidCopyOriginalAutomaticInputs(t *testing.T) {
	directory := os.Getenv("HYJS_P6_ACTUAL_COPY_PROTO_OUTPUT")
	playback := os.Getenv("HYJS_P6_ACTUAL_COPY_PROTO_PLAYBACK_OUTPUT") != ""
	if playback {
		directory = os.Getenv("HYJS_P6_ACTUAL_COPY_PROTO_PLAYBACK_OUTPUT")
	}
	if directory == "" {
		t.Skip("explicit actual paid copy native input export disabled")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	evidenceRoot, resolveErr := filepath.EvalSymlinks(filepath.Join(root, "analysis/p6"))
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	directory, err = filepath.Abs(directory)
	if err != nil || !strings.HasPrefix(directory, evidenceRoot+string(filepath.Separator)) {
		t.Fatal("owned P6 native evidence directory required", err)
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	var cases []map[string]any
	for index, item := range []struct{ task, protoSHA, jsonSHA string }{
		{"10", "5bf9b6c128da80c9b0b9812e5b60456d583b866c22d329b40f3704b0bf20ccae", "cb94cf48ae7facf8b250ccbdb1fd8da5291ffb8d95b2e0fd11cc83ad05ac2890"},
		{"12", "502cc6a0765aee667797bb8fd56c9ad4060afc39b406d3e24958de7b54e25d01", "28d8052507cd1f6d8fc56fc437b1c4cc601ca8710cfd03d6e3eb4e2bfc9ef04d"},
	} {
		prefix := filepath.Join(root, "analysis/p15/actual-copy-starts-20261015/mission3-task"+item.task+".SCCopyFightStart")
		raw, readErr := os.ReadFile(prefix + ".pb")
		if readErr != nil || p6Digest(raw) != item.protoSHA {
			t.Fatal("actual paid original proto representation", readErr)
		}
		start := &pb.SCCopyFightStart{}
		if err = proto.Unmarshal(raw, start); err != nil {
			t.Fatal(err)
		}
		raw, readErr = os.ReadFile(prefix + ".source.json")
		if readErr != nil || p6Digest(raw) != item.jsonSHA {
			t.Fatal("actual paid source object", readErr)
		}
		object := &pb.SCCopyFightStart{}
		if err = json.Unmarshal(raw, object); err != nil || !proto.Equal(start, object) || len(start.BattleMonsterList) != 1 {
			t.Fatal("actual paid source/proto semantic binding", err)
		}
		wire := p6ActualCopyPlaybackProjection(start)

		if playback {
			recordPath := strings.TrimSuffix(prefix, ".SCCopyFightStart") + ".OriginalOperates.pb"
			recordRaw, recordErr := os.ReadFile(recordPath)
			if recordErr != nil || p6Digest(recordRaw) != "876a8702571500c345fee44104993887c244ab979a776bc69e7c6245eb4cfa12" {
				t.Fatal("actual accepted record SHA", recordErr)
			}
			record := &pb.BattleOperate{}
			if err = proto.Unmarshal(recordRaw, record); err != nil {
				t.Fatal(err)
			}
			wire.OptRecord = record
		}
		raw, err = proto.MarshalOptions{Deterministic: true}.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		name := []string{"000.pb", "001.pb"}[index]
		if err = os.WriteFile(filepath.Join(directory, name), raw, 0644); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"case": "actual-paid-copy/chapter1/mission3/task" + item.task, "wire": name, "wireSHA256": p6Digest(raw), "fightId": wire.GetFightid(), "seed": wire.GetRandSeed(), "battleType": 1, "firstCamp": 0, "sourceOriginalStart": strings.TrimPrefix(prefix+".pb", root+string(filepath.Separator)), "sourceOriginalStartSHA256": item.protoSHA, "sourcePaidJSON": strings.TrimPrefix(prefix+".source.json", root+string(filepath.Separator)), "sourcePaidJSONSHA256": item.jsonSHA, "status": "awaiting-original-automatic-ai", "sourceBoundary": "actual ID4 paid10stamina natural original phone input; originalProto canonical encoding of stored start object, independent socket bytes not captured; same numeric actors/seed/max, no opts/winner/fixture injection"})
		cases[len(cases)-1]["damageFactors"] = map[string]any{"attacker": 1, "defender": 1, "source": "original BattleData.Clear0048..0060; SetData(SCCopyFightStart) preserves both; explicit SCFightPlayback projection avoids scalar default0"}

		if playback {
			row := cases[len(cases)-1]
			row["status"] = "awaiting-original-playback-ai"
			row["sourceOriginalRecord"] = strings.TrimPrefix(strings.TrimSuffix(prefix, ".SCCopyFightStart")+".OriginalOperates.pb", root+string(filepath.Separator))
			row["sourceOriginalRecordSHA256"] = "876a8702571500c345fee44104993887c244ab979a776bc69e7c6245eb4cfa12"
			row["sourceBoundary"] = "actual ID4 paid start + actual accepted CSCopyFightEnd player-only record stored sameTX; canonical objects/protobufs, socket bytes not independently captured; no predicted record or serverwinner"
		}
	}
	result := map[string]any{"scope": "Actual paid chapter3 task10/12 exact actors/seed/HP/pose/max, strict original PVE automatic read-only validation. No serverwinner or optRecord supplied. Original receiver native finalrecord read through original GetRecordOperates is separately recorded. Canonical serverstart object/source is genuine; socket bytes not independently captured.", "nativeCompared": false, "authoritative": false, "cases": cases}

	if playback {
		result["scope"] = "Actual paid chapter3 task10/12 source actors/HP/pose/seed/max + actual accepted source OriginalOperates, strict original PVE PlaybackAI readonly comparison. No Go predictedrecord/winner. Canonical original server/client objects, socket bytes not independently captured."
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "manifest.json"), append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("actual paid copy strict original input manifest", p6Digest(append(raw, '\n')))
}
