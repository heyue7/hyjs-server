package service

import (
	"fmt"
	"os"
	"testing"
)

func TestP6NativePetGuardMain12ID4Diagnostics(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") != "1" {
		t.Skip("explicit real-record diagnostic disabled")
	}
	for _, sample := range p6GuardNativeSamples() {
		t.Run(fmt.Sprint(sample.task), func(t *testing.T) {
			name := fmt.Sprintf("guard-main12-task%d-id4-native-20261013", sample.task)
			original, native := p6NativeMainFixture(t, name+".json", name+"-input.json", sample.id)
			p6CollectNativeMainDiagnostic(t, original, native, sample.fight, name+"-diagnostic.json")
		})
	}
}

type p6GuardNativeSample struct {
	task      int
	id, fight string
}

func p6GuardNativeSamples() []p6GuardNativeSample {
	return []p6GuardNativeSample{
		{5, "c869095ef128042507b2423eb2ff604dbe12511b5d2542ca45ce468a50ee5968", "7137722e6f71824ae0690c3f"},
		{7, "4502afd663907207474f9853ebb1ac5cf5fc4e40b2b0cde7c44110222c432882", "77584c56d1fe120aba8dae2e"},
		{8, "0dbf86b8a8229767a45e74193722ee0e91f67430744ad4846c93860845a217ca", "0f26a8c2b4736681d73b34ef"},
	}
}
