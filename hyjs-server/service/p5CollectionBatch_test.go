package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync/atomic"
	"testing"
)

// Eligibility remains exactly the first original Hero row / non99 rule. The
// test counts database calls rather than relying on machine-specific timings.
func TestP5CollectionBatchOriginalEligibilityHistoryAndRollback(t *testing.T) {
	f := p1Setup(t)
	if e := db.GetIns().AutoMigrate(&db.P5Collection{}); e != nil {
		t.Fatal(e)
	}
	var queries, creates atomic.Int64
	d := db.GetIns()
	if e := d.Callback().Query().After("gorm:query").Register("p5_collection_query_count", func(tx *gorm.DB) {
		if tx.Statement.Table == "p5_collections" {
			queries.Add(1)
		}
	}); e != nil {
		t.Fatal(e)
	}
	if e := d.Callback().Create().After("gorm:create").Register("p5_collection_create_count", func(tx *gorm.DB) {
		if tx.Statement.Table == "p5_collections" {
			creates.Add(1)
		}
	}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		d.Callback().Query().Remove("p5_collection_query_count")
		d.Callback().Create().Remove("p5_collection_create_count")
	})
	f.s.Tables["Hero"] = []map[string]string{{"id": "hero_001", "heroClass": "0"}, {"id": "hero_043", "heroClass": "100"}, {"id": "fixture_material_eligible", "heroClass": "0"}, {"id": "hero_099", "heroClass": "99"}, {"id": "duplicate_first_excluded", "heroClass": "99"}, {"id": "duplicate_first_excluded", "heroClass": "0"}}
	g := &model.GameState{Warriors: []*pb.Warrior{{Id: proto.String("hero_001")}, {Id: proto.String("hero_043")}, {Id: proto.String("fixture_material_eligible")}, {Id: proto.String("hero_099")}, {Id: proto.String("duplicate_first_excluded")}, {Id: proto.String("unknown")}, nil}}
	for i := 0; i < 5000; i++ {
		g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String("hero_exp_006")})
	}
	if e := d.Transaction(func(tx *gorm.DB) error { return f.s.ObserveP5Collection(tx, f.id, g) }); e != nil {
		t.Fatal(e)
	}
	if creates.Load() != 1 {
		t.Fatal("new eligible IDs must share one insert", creates.Load())
	}
	var rows []db.P5Collection
	if e := d.Where("player_id=?", f.id).Order("hero_id").Find(&rows).Error; e != nil {
		t.Fatal(e)
	}
	if len(rows) != 3 || rows[0].HeroID != "fixture_material_eligible" || rows[1].HeroID != "hero_001" || rows[2].HeroID != "hero_043" {
		t.Fatal("first original row eligibility changed", rows)
	}
	before := append([]db.P5Collection(nil), rows...)
	creates.Store(0)
	queries.Store(0)
	if e := d.Transaction(func(tx *gorm.DB) error { return f.s.ObserveP5Collection(tx, f.id, g) }); e != nil {
		t.Fatal(e)
	}
	if creates.Load() != 0 {
		t.Fatal("already observed ownership issued inserts", creates.Load())
	}
	if e := d.Where("player_id=?", f.id).Order("hero_id").Find(&rows).Error; e != nil {
		t.Fatal(e)
	}
	for i, row := range rows {
		if row != before[i] {
			t.Fatal("historical acquisition timestamp changed")
		}
	}
	// A failed outer save must not manufacture handbook history.
	f.s.Tables["Hero"] = append(f.s.Tables["Hero"], map[string]string{"id": "hero_new", "heroClass": "0"})
	g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String("hero_new")})
	rollback := errors.New("isolated rollback")
	if e := d.Transaction(func(tx *gorm.DB) error {
		if e := f.s.ObserveP5Collection(tx, f.id, g); e != nil {
			return e
		}
		return rollback
	}); !errors.Is(e, rollback) {
		t.Fatal(e)
	}
	var count int64
	if e := d.Model(&db.P5Collection{}).Where("player_id=? AND hero_id=?", f.id, "hero_new").Count(&count).Error; e != nil || count != 0 {
		t.Fatal("rollback left collection", e, count)
	}
	// An empty live bag retains historical membership and does not issue writes.
	creates.Store(0)
	if e := d.Transaction(func(tx *gorm.DB) error { return f.s.ObserveP5Collection(tx, f.id, &model.GameState{}) }); e != nil {
		t.Fatal(e)
	}
	if creates.Load() != 0 {
		t.Fatal("removal rewrote history")
	}
}
