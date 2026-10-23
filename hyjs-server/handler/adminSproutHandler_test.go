package handler

import (
	"hyjs-server/model"
	"strings"
	"testing"
)

func TestSproutSettingsValidation55(t *testing.T) {
	good := int64(14)
	zero := int64(0)
	over := int64(3651)
	for _, tc := range []struct {
		name    string
		enabled bool
		days    *int64
		want    bool
	}{
		{"unconfigured", false, nil, true}, {"enabled missing", true, nil, false}, {"configured", true, &good, true}, {"zero", false, &zero, false}, {"over", false, &over, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validAdminSproutSettings(model.AdminSproutSettingsSaveRequest{Enabled: tc.enabled, Days: tc.days, Version: strings.Repeat("a", 64), Reason: "测试"}); got != tc.want {
				t.Fatalf("validation=%v", got)
			}
		})
	}
}
