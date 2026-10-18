package service

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP10SceneReceiptCurrentLineOnly(t *testing.T) {
	f, b, _ := p10SceneSetup(t)
	a := f.id
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, a, ra)
	p10SceneConnect(t, f, b, rb)
	var before db.Profile
	if err := db.GetIns().First(&before, a).Error; err != nil {
		t.Fatal(err)
	}
	toTwo := &pb.CSTransferLine{Line: proto.Int32(2)}
	toOne := &pb.CSTransferLine{Line: proto.Int32(1)}
	for i, request := range []*pb.CSTransferLine{toTwo, toOne} {
		if _, handled, err := f.s.DispatchP10Scene(f.ctx, a, request, int64(4101+i)); err != nil || !handled {
			t.Fatal(handled, err)
		}
	}
	ra.drain()
	rb.drain()
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, a, toTwo, 4101); err == nil || !handled {
		t.Fatal("old line-two success replayed after actual return to line one", handled, err)
	}
	// Same current-line retries share the durable receipt and never publish new visibility.
	var workers sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, _, err := f.s.DispatchP10Scene(f.ctx, a, toOne, 4102)
			errors <- err
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(ra.drain())+len(rb.drain()) != 0 {
		t.Fatal("receipt retries changed peer visibility")
	}
	var own db.P10ScenePresence
	var count int64
	var after db.Profile
	if err := db.GetIns().First(&own, "player_id=?", a).Error; err != nil || own.Line != 1 {
		t.Fatal("receipt changed current line", own.Line, err)
	}
	if err := db.GetIns().Model(&db.P10SocialReceipt{}).Where("player_id=? AND domain=?", a, "line-transfer").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("receipt cardinality", count, err)
	}
	if err := db.GetIns().First(&after, a).Error; err != nil || *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("line retries changed profile/resources", err)
	}
}

func TestP10SceneOriginalTCPNoTimestamp(t *testing.T) {
	f, b, _ := p10SceneSetup(t)
	a := f.id
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, a, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	for _, line := range []int32{2, 1} {
		request := &pb.CSTransferLine{Line: proto.Int32(line)}
		if _, handled, err := f.s.DispatchP10Scene(f.ctx, a, request, 0); err != nil || !handled {
			t.Fatal("original TCP zero timestamp transfer rejected", handled, err)
		}
		ra.drain()
		rb.drain()
		for i := 0; i < 8; i++ {
			if _, _, err := f.s.DispatchP10Scene(f.ctx, a, request, 0); err != nil {
				t.Fatal(err)
			}
		}
		if len(ra.drain())+len(rb.drain()) != 0 {
			t.Fatal("same-line zero timestamp repeated visibility")
		}
	}
	var count int64
	if err := db.GetIns().Model(&db.P10SocialReceipt{}).Where("player_id=? AND domain=?", a, "line-transfer").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("original TCP must not manufacture timestamp receipts", count, err)
	}
	if _, _, err := f.s.DispatchP10Scene(f.ctx, a, &pb.CSTransferLine{Line: proto.Int32(2)}, -1); err == nil {
		t.Fatal("negative timestamp accepted")
	}
}
