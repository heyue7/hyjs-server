package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// A seasonal child can have an arbitrary valid event kind; its first freeze
// must retry before taking catalogue X, including the parent directory write.
func TestP1GatherSeasonalFirstFreezeRetries(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	suffix := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	parentID := "update_p4_" + suffix
	periodID := "totalLoginUpdate_p4_update_" + suffix
	if err := db.GetIns().Model(&db.P15SeasonalBundle{}).Where("id=?", parentID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", periodID).Updates(map[string]any{"enabled": true, "kind": constUtil.P8EventMineGather}).Error; err != nil {
		t.Fatal(err)
	}
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var live db.PeriodInstance
		if err := tx.First(&live, "id=?", periodID).Error; err != nil {
			return err
		}
		_, err := f.s.freezeP1Period(tx, live, true)
		return err
	})
	if !errors.Is(err, constUtil.ErrP1GatherCatalogWriteRetry) {
		t.Fatalf("unguarded first freeze was not rejected: %v", err)
	}
	result, err := f.s.p15SeasonalList(f.ctx, f.id, &pb.CSActivityList{ActivityId: proto.String(parentID)}, now)
	if err != nil || len(result) != 1 {
		t.Fatalf("seasonal directory did not retry: %v", err)
	}
	out, ok := result[0].(*pb.SCActivityList)
	if !ok || len(out.GetActivityList()) != 1 || out.GetActivityList()[0].GetActivityId() != periodID {
		t.Fatal("seasonal directory did not preserve its child")
	}
	var live, frozen db.PeriodInstance
	if err = db.GetIns().First(&live, "id=?", periodID).Error; err != nil || json.Unmarshal(live.Frozen, &frozen) != nil || frozen.Kind != constUtil.P8EventMineGather {
		t.Fatalf("seasonal retry did not freeze the same kind: %v", err)
	}
}
