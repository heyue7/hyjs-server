package model

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

// P5GuildPrayerProjection is a transaction-local attribute source. GameState
// excludes it from JSON so a later request must recheck membership and expiry.
type P5GuildPrayerProjection struct {
	PlayerID       int64
	Properties     *pb.BaseProp
	SourceHash     string
	CapturedAt     int64
	ExpiresAt      int64
	TechProperties *pb.BaseProp
	TechSourceHash string
}

// Clone preserves this immutable transaction source across an internal reward
// JSON clone while keeping subsequent snapshots independent of mutable PBs.
func (p *P5GuildPrayerProjection) Clone() *P5GuildPrayerProjection {
	if p == nil {
		return nil
	}
	copy := *p
	if p.Properties != nil {
		copy.Properties = proto.Clone(p.Properties).(*pb.BaseProp)
	}
	if p.TechProperties != nil {
		copy.TechProperties = proto.Clone(p.TechProperties).(*pb.BaseProp)
	}
	return &copy
}
