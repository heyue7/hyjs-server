package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOLeaveNativeReceiptAtomicAndPositionBound(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	if err := db.GetIns().AutoMigrate(&db.FieldBossContribution73{}); err != nil {
		t.Fatal(err)
	}
	id := f.ids[0]
	requestID := "00000000-0000-4000-8000-000000000091"
	prepared, err := f.s.GGOFieldLeavePrepare(f.ctx, model.GGOFieldLeavePrepareRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: requestID}, Mode: "scene", TargetScene: "town",
	}, f.sessions[0])
	if err != nil || prepared.Status != "prepared" {
		t.Fatalf("prepare: %v %v", prepared, err)
	}
	runtime := f.s.withRuntimeTables(f.s.Tables)
	runtime.GGOFieldEconomy = f.config
	native := &pb.CSTransferScene{Target: proto.String("town")}
	reply := &pb.SCTransferScene{Result: proto.Int32(1)}
	rollback := errors.New("forced rollback")
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.First(&row, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		if err = ggoCommitLeave(tx, runtime, id, game, "scene", "town", 0, time.Now().UnixMilli(), native, reply); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("rollback path", err)
	}
	statusReq := model.GGOFieldLeaveStatusRequest{GGOFieldAuth: f.auth(0), LeaveRequestID: requestID}
	status, err := f.s.GGOFieldLeaveStatus(f.ctx, statusReq, f.sessions[0])
	if err != nil || status.Status != "prepared" {
		t.Fatalf("rolled back leave was visible: %v %v", status, err)
	}
	messages, err := runtime.fieldTransfer(f.ctx, id, native)
	if err != nil || len(messages) != 1 {
		t.Fatalf("native transfer: %v", err)
	}
	actual, ok := messages[0].(*pb.SCTransferScene)
	if !ok {
		t.Fatal("wrong native response")
	}
	requestWire, err := proto.MarshalOptions{Deterministic: true}.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	status, err = f.s.GGOFieldLeaveStatus(f.ctx, statusReq, f.sessions[0])
	if err != nil || status.Status != "completed" || !status.TargetCurrent || status.PositionRevision < 1 || status.NativeRequestHash != p6Digest(requestWire) || status.Settlement == nil {
		t.Fatalf("committed receipt: %v %v", status, err)
	}
	var recovered pb.SCTransferScene
	if err = proto.Unmarshal(status.NativeResponse, &recovered); err != nil || !proto.Equal(&recovered, actual) {
		t.Fatalf("wrong recovered native response: %v", err)
	}
	if status.Settlement.CreditAdded != 30 || status.Settlement.CreditBalance != 30 {
		t.Fatalf("wrong settlement: %v", status.Settlement)
	}
	// A later trip back to the same scene must not revive this old callback.
	if _, err = runtime.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town")}); err != nil {
		t.Fatal(err)
	}
	status, err = f.s.GGOFieldLeaveStatus(f.ctx, statusReq, f.sessions[0])
	if err != nil || status.TargetCurrent {
		t.Fatalf("stale callback became valid again: %v %v", status, err)
	}
}

func TestGGOLeaveNativeLineEmptyResponse(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	if err := db.GetIns().AutoMigrate(&db.FieldBossContribution73{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	requestID := "00000000-0000-4000-8000-000000000092"
	_, err := f.s.GGOFieldLeavePrepare(f.ctx, model.GGOFieldLeavePrepareRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: requestID}, Mode: "line", TargetLine: 2,
	}, f.sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	runtime := f.s.withRuntimeTables(f.s.Tables)
	runtime.GGOFieldConfig, runtime.GGOFieldEconomy = f.config, f.config
	request := &pb.CSTransferLine{Line: proto.Int32(2)}
	messages, handled, err := runtime.DispatchP10Scene(f.ctx, f.ids[0], request, 0)
	if err != nil || !handled || len(messages) != 1 {
		t.Fatalf("native line transfer: %v", err)
	}
	if _, ok := messages[0].(*pb.SCTransferLine); !ok {
		t.Fatal("wrong native line response")
	}
	status, err := f.s.GGOFieldLeaveStatus(f.ctx, model.GGOFieldLeaveStatusRequest{GGOFieldAuth: f.auth(0), LeaveRequestID: requestID}, f.sessions[0])
	if err != nil || status.Status != "completed" || !status.TargetCurrent || status.PositionRevision < 1 || status.NativeResponse == nil || len(status.NativeResponse) != 0 {
		t.Fatalf("line receipt: %v %v", status, err)
	}
	encoded, err := json.Marshal(status)
	if err != nil || !bytes.Contains(encoded, []byte(`"nativeResponse":""`)) {
		t.Fatalf("empty protobuf was lost: %s %v", encoded, err)
	}
}
