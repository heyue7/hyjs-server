package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
)

func Trial87Prepare(raw []byte, dir string) (any, error) {
	var series model.FrozenP6RushSeries
	if e := json.Unmarshal(raw, &series); e != nil {
		return nil, e
	}
	i := series.Template
	s := &GameService{Tables: i.Tables}
	out, e := p7GenericMechanismRushPreview(context.Background(), s, &series)
	if e != nil {
		return nil, e
	}
	wire := &pb.SCFightPlayback{Attacker: i.Attacker, Defender2: i.Monster, RandSeed: proto.Int64(i.Seed), MaxRound: proto.Int32(i.MaxRound), BattleType: proto.Int32(i.BattleType), Fightid: series.Start.FightId, AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
	w, e := proto.Marshal(wire)
	if e != nil {
		return nil, e
	}
	b, _ := proto.Marshal(series.Start)
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "000.pb"), w, 0600)
	os.WriteFile(filepath.Join(dir, "000.rush-start.bin"), b, 0600)
	o, _ := json.Marshal(out)
	os.WriteFile(filepath.Join(dir, "automatic.json"), o, 0600)
	o, _ = proto.Marshal(out.OriginalOperates)
	os.WriteFile(filepath.Join(dir, "go-record.pb"), o, 0600)
	var controls []map[string]any
	for n := int32(1); n <= i.MaxRound; n += 2 {
		controls = append(controls, map[string]any{"roundNum": n, "roundTime": 0})
	}
	o, _ = json.Marshal(controls)
	os.WriteFile(filepath.Join(dir, "controls.json"), o, 0600)
	return map[string]any{"battleType": i.BattleType, "maxRound": i.MaxRound, "ops": len(out.OriginalOperates.SkillOpts), "round": out.OriginalOperates.GetFinalRoundNum(), "attackerHP": out.OriginalOperates.GetAttackerTotalLeftHp(), "defenderHP": out.OriginalOperates.GetDefenderTotalLeftHp()}, nil
}
func Trial87Compare(raw, rec []byte) (any, error) {
	var series model.FrozenP6RushSeries
	if e := json.Unmarshal(raw, &series); e != nil {
		return nil, e
	}
	record := &pb.BattleOperate{}
	if e := proto.Unmarshal(rec, record); e != nil {
		return nil, e
	}
	s := &GameService{Tables: series.Template.Tables}
	result := map[string]any{}
	add := func(name string, o *model.CombatFinalHPOutcome, e error) {
		r := map[string]any{}
		if e != nil {
			r["error"] = e.Error()
		}
		if o != nil && o.OriginalOperates != nil {
			p := o.OriginalOperates
			r["exact"] = proto.Equal(p, record)
			r["ops"] = len(p.SkillOpts)
			r["round"] = p.GetFinalRoundNum()
			r["attackerHP"] = p.GetAttackerTotalLeftHp()
			r["defenderHP"] = p.GetDefenderTotalLeftHp()
			for n := 0; n < len(p.SkillOpts) && n < len(record.SkillOpts); n++ {
				a, b := p.SkillOpts[n], record.SkillOpts[n]
				if !proto.Equal(a, b) {
					r["firstSkillDifference"] = map[string]any{"index": n, "expectedRound": b.GetRoundNum(), "actualRound": a.GetRoundNum(), "expectedTime": b.GetRoundTime(), "actualTime": a.GetRoundTime(), "expectedSkill": b.GetSkillIndex(), "actualSkill": a.GetSkillIndex(), "sameActor": b.GetActorUid() == a.GetActorUid()}
					break
				}
			}
		}
		result[name] = r
	}
	o, e := p7GenericMechanismRushPreview(context.Background(), s, &series)
	add("automatic", o, e)
	o, e = s.computeFrozenBattleFinalHPLiveControlled(context.Background(), series.Template, record, true, nil, nil, true)
	add("liveControlledRaw", o, e)
	reported := int32(0)
	if record.GetDefenderTotalLeftHp() <= 0 {
		reported = 1
	}
	o, e = p7GenericMechanismRushComputeEnd(context.Background(), s, &series, record, reported)
	add("actualEnd", o, e)
	o, e = p7LiveRecordedRushComputeEnd(context.Background(), s, &series, record, reported)
	add("liveRecordedEnd", o, e)
	return result, nil
}
