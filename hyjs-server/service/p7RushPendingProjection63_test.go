package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestP7Rush63PendingPowerUsesFrozenStartAndRetainsRule(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(2)}, 630019, false)
	if err != nil {
		t.Fatal(err)
	}
	var before db.P7RushFight
	if err = db.GetIns().First(&before, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var frozen db.P7RushStageRule
	if json.Unmarshal(before.Rule, &frozen) != nil || len(frozen.Enemies) != 0 || frozen.RecommendedPower != 0 {
		t.Fatal("historical rule must omit enemy bytes and lack a score")
	}
	// Replaced live catalogue must not supply the old fight's display or identity.
	if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("source=?", constUtil.P7SectionStageSource).Updates(map[string]any{"enabled": false, "enemies": []byte("invalid-current-enemy"), "recommended_power": 999999999}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	current, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tasks := current[0].(*pb.SCRushCurrentInfo).GetTaskInfoList()
	if len(tasks) != 1 || tasks[0].GetStar() != 2 || tasks[0].GetFightScore() <= 0 || tasks[0].GetFightScore() == 999999999 {
		t.Fatal("pending Ready did not use its real frozen enemy", tasks)
	}
	var after db.P7RushFight
	if err = db.GetIns().First(&after, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Rule, after.Rule) || before.RuleHash != after.RuleHash || !bytes.Equal(before.Start, after.Start) || before.StartHash != after.StartHash {
		t.Fatal("display changed immutable fight evidence")
	}
	// Missing/tampered frozen evidence continues to reject; live catalogue cannot repair it.
	if err = db.GetIns().Model(&db.P7RushFight{}).Where("id=?", start.GetFightId()).Update("start", append(append([]byte(nil), before.Start...), 0)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now()); err == nil {
		t.Fatal("tampered frozen start accepted")
	}
}
