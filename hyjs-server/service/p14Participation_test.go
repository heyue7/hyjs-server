package service

import (
	"hyjs-server/constUtil"
	"testing"
)

func TestP14OriginalParticipationZeroAdmittedStageCodeEliminatedAndPendingNeverBooleanOne(t *testing.T) {
	for _, stage := range []int32{2, 3, 4, 5, 6, 7, 8} {
		code, err := p14OriginalParticipation(stage, true, true, 0)
		if err != nil || code != 0 {
			t.Fatal("original promotion/finals UI hides nonzero participation", stage, code, err)
		}
	}
	for _, stage := range []int32{3, 5, 6, 7, 8, 9, 19} {
		if _, err := p14OriginalParticipation(stage, true, false, 0); err == nil {
			t.Fatal("pending facts became a wire boolean or fabricated elimination", stage)
		}
	}
	for _, failed := range []int32{4, 6, 8, 10, 12, 14, 16, 18} {
		code, err := p14OriginalParticipation(19, true, false, failed)
		if err != nil || code != failed {
			t.Fatal("original log fail_stage switch", failed, err)
		}
	}
	for _, failed := range []int32{1, 2, 3, 5, 7, 9, 19} {
		if _, err := p14OriginalParticipation(19, true, false, failed); err == nil {
			t.Fatal("unknown original elimination code", failed)
		}
	}
	if _, err := p14OriginalParticipation(5, true, false, constUtil.P14EliminatedPromotion); err == nil {
		t.Fatal("future promotion failure")
	}
	if _, err := p14OriginalParticipation(7, true, true, constUtil.P14EliminatedPromotion); err == nil {
		t.Fatal("simultaneous admitted and eliminated")
	}
	if code, err := p14OriginalParticipation(7, false, false, 0); err != nil || code != 0 {
		t.Fatal("unregistered uses real Login.isEnter, never fake elimination", err)
	}
}
