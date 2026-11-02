package service

import (
	"strings"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func ackIdentityRound2Packet(f *p10PushAuthFixture) *model.GamePacket {
	return &model.GamePacket{
		Opcode:   30038,
		PlayerId: f.profile.Id,
		ZoneId:   int32(f.s.OriginalServerID()),
		Session:  append([]byte(nil), f.profile.Session...),
		Body:     &pb.CSMoveConfirm{},
	}
}

func TestAckIdentityRound2SQLite(t *testing.T) {
	t.Run("valid acknowledgement reads only identity", func(t *testing.T) {
		f := p10PushAuthSetup(t)
		if f.d.Dialector.Name() != "sqlite" {
			t.Fatal("fixture must use isolated SQLite")
		}
		var profileQueries []string
		callback := f.d.Callback().Query()
		const callbackName = "ack_identity_round2_profiles"
		if err := callback.After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table == "profiles" {
				profileQueries = append(profileQueries, tx.Statement.SQL.String())
			}
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = callback.Remove(callbackName) })

		out := f.s.Dispatch(f.ctx, ackIdentityRound2Packet(f), "isolated-ack")
		if len(out) != 0 {
			t.Fatalf("valid original one-way acknowledgement returned %d messages", len(out))
		}
		if len(profileQueries) != 1 {
			t.Fatalf("expected exactly one profile read, got %d", len(profileQueries))
		}
		query := strings.NewReplacer("`", "", "\"", "", "[", "", "]", "").Replace(strings.ToLower(profileQueries[0]))
		projection, _, found := strings.Cut(query, " from ")
		if !found || !strings.HasPrefix(projection, "select ") {
			t.Fatalf("unexpected profile query: %s", profileQueries[0])
		}
		selected := strings.Split(strings.TrimPrefix(projection, "select "), ",")
		wanted := map[string]bool{"id": true, "server_id": true, "session": true, "account_id": true, "account_auth_revision": true}
		if len(selected) != len(wanted) {
			t.Fatalf("profile query selected unexpected columns: %s", projection)
		}
		for _, field := range selected {
			field = strings.TrimSpace(field)
			if dot := strings.LastIndex(field, "."); dot >= 0 {
				field = field[dot+1:]
			}
			if !wanted[field] {
				t.Fatalf("profile query selected unexpected column %q: %s", field, projection)
			}
			delete(wanted, field)
		}
		if len(wanted) != 0 {
			t.Fatalf("profile identity query omitted fields: %v", wanted)
		}
	})

	cases := []struct {
		name    string
		prepare func(*testing.T, *p10PushAuthFixture, *model.GamePacket)
		code    string
	}{
		{"expired session", func(t *testing.T, f *p10PushAuthFixture, packet *model.GamePacket) {
			packet.Session = []byte("fedcba9876543210")
		}, constUtil.ErrGameSession},
		{"disabled account", func(t *testing.T, f *p10PushAuthFixture, packet *model.GamePacket) {
			if err := f.d.Model(&db.GameAccount{}).Where("id=?", f.account.ID).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
		}, constUtil.ErrGameSession},
		{"revoked account revision", func(t *testing.T, f *p10PushAuthFixture, packet *model.GamePacket) {
			if err := f.d.Model(&db.GameAccount{}).Where("id=?", f.account.ID).Update("auth_revision", f.account.AuthRevision+1).Error; err != nil {
				t.Fatal(err)
			}
		}, constUtil.ErrGameSession},
		{"client update gate", func(t *testing.T, f *p10PushAuthFixture, packet *model.GamePacket) {
			if err := f.d.Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("min_run_build", f.session.BuildCode+1).Error; err != nil {
				t.Fatal(err)
			}
		}, constUtil.ErrGameSession},
		{"wrong request zone", func(t *testing.T, f *p10PushAuthFixture, packet *model.GamePacket) {
			packet.ZoneId++
		}, constUtil.ErrGameRequest},
		{"wrong profile zone", func(t *testing.T, f *p10PushAuthFixture, packet *model.GamePacket) {
			if err := f.d.Model(&db.Profile{}).Where("id=?", f.profile.Id).Update("server_id", f.s.OriginalServerID()+1).Error; err != nil {
				t.Fatal(err)
			}
		}, constUtil.ErrGameRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := p10PushAuthSetup(t)
			if f.d.Dialector.Name() != "sqlite" {
				t.Fatal("fixture must use isolated SQLite")
			}
			packet := ackIdentityRound2Packet(f)
			tc.prepare(t, f, packet)
			out := f.s.Dispatch(f.ctx, packet, "isolated-ack")
			if len(out) != 1 {
				t.Fatalf("rejected acknowledgement returned %d messages", len(out))
			}
			message, ok := out[0].(*pb.ErrorMessage)
			if !ok || message.GetCode() != tc.code {
				t.Fatalf("wrong acknowledgement rejection: %v", out)
			}
		})
	}
}
