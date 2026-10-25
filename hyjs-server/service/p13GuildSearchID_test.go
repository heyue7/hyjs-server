package service

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func TestP13GuildSearchActualCreatedID(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	now := time.Now()
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("检索公会")}, 1, now)
	id := p13GuildID(t, f, ids[0], now)
	for _, query := range []string{id, strings.ToUpper(id), "检索公会"} {
		result, err := f.s.p13GuildQuery(f.ctx, ids[1], &pb.CSUnionSearch{Page: proto.Int32(1), Name: proto.String(query)}, now)
		if err != nil {
			t.Fatal(query, err)
		}
		unions := result[0].(*pb.SCUnionSearch).Unions
		if len(unions) != 1 || unions[0].GetUnionId() != id {
			t.Fatal("created guild must be searchable by its exact ID and name", query, unions)
		}
	}
	if _, err := f.s.p13GuildQuery(f.ctx, ids[1], &pb.CSUnionSearch{Page: proto.Int32(1), Name: proto.String(strings.Repeat("剑", 7))}, now); err == nil {
		t.Fatal("over-budget name accepted")
	}
}
