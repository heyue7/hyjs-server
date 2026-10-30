package service

import (
	"errors"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestGGOClientPatrolProtocolBuildFloor(t *testing.T) {
	for _, tc := range []struct {
		name              string
		configured, build int64
		allowed           bool
	}{
		{"current_release_rejected", 2026102904, 2026102904, false},
		{"one_build_below_protocol_rejected", 2026102904, 2026103101, false},
		{"new_protocol_admitted", 2026102904, 2026103102, true},
		{"later_build_admitted", 2026102904, 2026103103, true},
		{"newer_config_still_enforced", 2026103103, 2026103102, false},
		{"newer_config_at_boundary", 2026103103, 2026103103, true},
		{"unconfigured_fails_closed", 0, 2026103102, false},
		{"negative_config_fails_closed", -1, 2026103102, false},
		{"unknown_build_rejected", 2026102904, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ggoClientBuildCompatible(tc.configured, tc.build); got != tc.allowed {
				t.Fatalf("configured=%d build=%d: got %t", tc.configured, tc.build, got)
			}
		})
	}
}

func TestGGOClientBuildRejectsInvalidConfigurationBeforeLookup(t *testing.T) {
	validSession := make([]byte, 16)
	for _, tc := range []struct {
		cfg     *model.AdminFieldGGOConfiguration
		session []byte
	}{
		{nil, validSession},
		{&model.AdminFieldGGOConfiguration{}, validSession},
		{&model.AdminFieldGGOConfiguration{Runtime: &model.AdminFieldGGORuntime{MinClientBuild: 0}}, validSession},
		{&model.AdminFieldGGOConfiguration{Runtime: &model.AdminFieldGGORuntime{MinClientBuild: 2026103102}}, []byte{1}},
	} {
		err := ggoCheckClientBuildLocked(nil, 1, tc.session, tc.cfg)
		var biz *model.BizCode
		if !errors.As(err, &biz) || biz.Code != constUtil.GGOFieldClientUpdateRequired {
			t.Fatalf("invalid input did not require an update: %v", err)
		}
	}
}
