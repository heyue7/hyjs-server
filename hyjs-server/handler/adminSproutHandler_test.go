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

func TestSproutReturnSettingsValidation73(t *testing.T) {
	yes, no := true, false
	days, invalid := int64(7), int64(0)
	req := model.AdminSproutSettingsSaveRequest{Enabled: true, NewCharacterEnabled: &no, ReturnEnabled: &yes, ReturnOfflineDays: &days, ReturnDays: &days, Version: strings.Repeat("a", 64), Reason: "回归配置"}
	if !validAdminSproutSettings(req) {
		t.Fatal("return-only rule rejected")
	}
	req.ReturnDays = nil
	if validAdminSproutSettings(req) {
		t.Fatal("missing return duration accepted")
	}
	req.ReturnEnabled = &no
	req.ReturnDays = &invalid
	if validAdminSproutSettings(req) {
		t.Fatal("invalid dormant duration accepted")
	}
}
