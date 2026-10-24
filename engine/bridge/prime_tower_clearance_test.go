package bridge

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPrimeTowerClearanceIsUnverifiedForMulticolorTarget(t *testing.T) {
	for _, count := range []int{1, 4} {
		t.Run(map[int]string{1: "single_material", 4: "four_materials"}[count], func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.3mf")
			output := filepath.Join(dir, "U1-source.3mf")
			fixture(t, source, count)
			report, err := Convert(source, output, "u1")
			if err != nil {
				t.Fatal(err)
			}
			needReview := count > 1
			if report["prime_tower_clearance_review_required"] != needReview {
				t.Fatalf("expected unverified target prime-tower clearance=%t: %+v", needReview, report)
			}
			if needReview {
				found := false
				for _, message := range report["warnings"].([]string) {
					if strings.Contains(message, "tower") && strings.Contains(message, "collision") {
						found = true
					}
				}
				if !found {
					t.Fatalf("tower collision warning absent from report: %+v", report)
				}
			}
		})
	}
}

func TestPrimeTowerWarningRequiresAnEnabledNativeTargetTower(t *testing.T) {
	cases := []struct {
		raw  any
		want bool
	}{
		{"1", true}, {"true", true}, {true, true}, {1, true}, {float64(1), true},
		{"0", false}, {"false", false}, {false, false}, {nil, false}, {"unknown", false},
	}
	for _, tc := range cases {
		got := primeTowerProfileEnabled(map[string]any{"enable_prime_tower": tc.raw})
		if got != tc.want {
			t.Fatalf("native enabled flag raw=%v => %t, want %t", tc.raw, got, tc.want)
		}
	}
}
