package service

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestP7FormationRecorded87OriginalStrictAndCold(t *testing.T) {
	s, _, _ := p7LiveRecordedOriginal34(t)
	raw, err := os.ReadFile("testdata/p7_formation_recorded87.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name         string
			StartHex     string
			RecordBase64 string
			Won          bool
		}
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Cases) != 3 {
		t.Fatal("fixtures", err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			start := &pb.SCRushFightStart{}
			data, _ := hex.DecodeString(c.StartHex)
			if proto.Unmarshal(data, start) != nil {
				t.Fatal("start")
			}
			series, err := s.FreezeP6RushSeriesInput(start, start.GetRushType() == 1, false, nil, constUtil.P6OriginalClientHash)
			if err != nil {
				t.Fatal(err)
			}
			record := &pb.BattleOperate{}
			data, _ = base64.StdEncoding.DecodeString(c.RecordBase64)
			if proto.Unmarshal(data, record) != nil {
				t.Fatal("record")
			}
			if _, err = s.ComputeP6RoundOneLiveAutoFormation(t.Context(), series.Template, record, record.AttackerTransOpt); err == nil {
				t.Fatal("native delayed source must cover missing old branch")
			}
			before, _ := json.Marshal(series)
			reported := int32(0)
			if c.Won {
				reported = 1
			}
			out, err := p7GenericMechanismRushComputeEnd(t.Context(), s, series, record, reported)
			if err != nil {
				t.Fatal("native legal formation+commands", err)
			}
			if out.Outcome.CapabilityHash != p7FormationRecordedProof || !proto.Equal(out.OriginalOperates, record) {
				t.Fatal("new proof or complete record")
			}
			body, _ := json.Marshal(out)
			var cold = *out
			if json.Unmarshal(body, &cold) != nil {
				t.Fatal("cold")
			}
			if p7GenericMechanismRushValidateEnd(series, &cold, record, reported) != nil {
				t.Fatal("cold receipt")
			}
			after, _ := json.Marshal(series)
			if !bytes.Equal(before, after) {
				t.Fatal("source mutated")
			}
			for _, field := range []string{"hp", "round", "clock", "actor", "slot", "duplicate", "late", "skill", "result"} {
				bad := proto.Clone(record).(*pb.BattleOperate)
				result := reported
				switch field {
				case "hp":
					bad.AttackerTotalLeftHp = proto.Float64(bad.GetAttackerTotalLeftHp() + 1)
				case "round":
					bad.FinalRoundNum = proto.Int32(bad.GetFinalRoundNum() + 1)
				case "clock":
					bad.SkillOpts[0].RoundTime = proto.Float64(-1)
				case "actor":
					bad.AttackerTransOpt.ActorUids[0] = "foreign"
				case "slot":
					bad.AttackerTransOpt.Fpids[0] = 16
				case "duplicate":
					bad.AttackerTransOpt.Fpids[0] = bad.AttackerTransOpt.Fpids[1]
				case "late":
					bad.AttackerTransOpt.RoundNum = proto.Int32(2)
				case "skill":
					bad.SkillOpts[0].SkillIndex = proto.Int32(99)
				case "result":
					result = 1 - reported
				}
				if _, err = p7GenericMechanismRushComputeEnd(t.Context(), s, series, bad, result); err == nil {
					t.Fatal("forged End accepted", field)
				}
			}
			if _, _, err = p7FormationRecordedSource(series.Template, record); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestP7RejectedEvidence87BoundedPrivateExpiry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	now := time.Now()
	var path string
	for i := 0; i < 8; i++ {
		var ok bool
		path, ok = p7WriteRejectedEvidence(dir, []byte{byte(i)}, now)
		if !ok {
			t.Fatal("write")
		}
	}
	files, _ := os.ReadDir(dir)
	if len(files) != p7RejectedEvidenceLimit {
		t.Fatal("quota", len(files))
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal("directory privacy")
	}
	info, _ = os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("file privacy")
	}
	old := now.Add(-p7RejectedEvidenceTTL - time.Minute)
	for _, f := range files {
		if err := os.Chtimes(filepath.Join(dir, f.Name()), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := p7WriteRejectedEvidence(dir, []byte("fresh"), now); !ok {
		t.Fatal("fresh")
	}
	files, _ = os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal("expiry")
	}
	if _, ok := p7WriteRejectedEvidence(dir, make([]byte, p7RejectedEvidenceMaxBytes+1), now); ok {
		t.Fatal("oversize")
	}
	link := filepath.Join(t.TempDir(), "link")
	if os.Symlink(dir, link) != nil {
		t.Fatal("symlink")
	}
	if _, ok := p7WriteRejectedEvidence(link, []byte("no"), now); ok {
		t.Fatal("symlink accepted")
	}
}
