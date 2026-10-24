package bridge

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const twoTowerPlatesXML = "<config><plate><metadata key='plater_id' value='1'/><model_instance><metadata key='object_id' value='1'/></model_instance></plate><plate><metadata key='plater_id' value='2'/><model_instance><metadata key='object_id' value='1'/></model_instance></plate></config>"

// This is deliberately only a structural count, never a clearance decision.
// Null, invalid and nonfinite slots cannot silently pass as configured.
func TestTowerPlatePositionAuditCountsMissingAndValidEntries(t *testing.T) {
	tests := []struct {
		name    string
		x, y    any
		wantX   int
		wantY   int
		missing bool
	}{
		{"absent", nil, nil, 0, 0, true},
		{"single_pair", []string{"30"}, []string{"30"}, 1, 1, true},
		{"two_valid_pairs", []string{"30", "30"}, []string{"30", "30"}, 2, 2, false},
		{"numeric_pair", []float64{0, 30}, []float64{30, 30}, 2, 2, false},
		{"only_y_incomplete", []string{"30", "30"}, []string{"30"}, 2, 1, true},
		{"nonfinite_y", []string{"30", "30"}, []string{"30", "NaN"}, 2, 1, true},
		{"sparse_x", []any{"30", nil, "30"}, []string{"30", "30"}, 1, 2, true},
		{"extra_unused_pair", []string{"30", "30", "30"}, []string{"30", "30", "30"}, 2, 2, false},
		{"scalar_not_two_plates", "30", "30", 1, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "two-plates.3mf")
			fixture(t, source, 4)
			rewriteTestSourceArchive(t, source, func(name string, content []byte) []byte {
				if name == "Metadata/model_settings.config" {
					return []byte(twoTowerPlatesXML)
				}
				if name != "Metadata/project_settings.config" {
					return content
				}
				settings := map[string]any{}
				if err := json.Unmarshal(content, &settings); err != nil {
					t.Fatal(err)
				}
				if tc.x != nil {
					settings["wipe_tower_x"] = tc.x
				}
				if tc.y != nil {
					settings["wipe_tower_y"] = tc.y
				}
				out, err := json.Marshal(settings)
				if err != nil {
					t.Fatal(err)
				}
				return out
			})
			result := auditTargetTowerPlatePositions(source)
			if result.plates != 2 || result.xEntries != tc.wantX ||
				result.yEntries != tc.wantY || result.incomplete() != tc.missing {
				t.Fatalf("got %+v (incomplete=%t); want plates=2 x=%d y=%d missing=%t",
					result, result.incomplete(), tc.wantX, tc.wantY, tc.missing)
			}
		})
	}
}

func TestSingleColorTwoPlateProjectDoesNotWarnAboutPrimeTowerPositions(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "single-color.3mf")
	output := filepath.Join(dir, "target.3mf")
	fixture(t, source, 1)
	rewriteTestSourceArchive(t, source, func(name string, content []byte) []byte {
		if name == "Metadata/model_settings.config" {
			return []byte(twoTowerPlatesXML)
		}
		return content
	})
	report, err := Convert(source, output, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if report["prime_tower_plate_position_audit_status"] != "not_applicable" ||
		report["prime_tower_clearance_review_required"] != false ||
		report["prime_tower_plate_coordinate_review_required"] != false {
		t.Fatalf("single-color should not get tower review: %+v", report)
	}
}

func TestUnknownPlateMetadataDoesNotClaimPositionsVerified(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "source.3mf")
	fixture(t, filename, 4) // No <plate> elements in metadata.
	audit := auditTargetTowerPlatePositions(filename)
	if audit.plates != 0 || audit.xEntries != 0 || audit.yEntries != 0 {
		t.Fatalf("unknown plate count cannot be invented: %+v", audit)
	}
}

func TestTowerPositionAuditStatusNeverMeansCollisionSafe(t *testing.T) {
	cases := []struct {
		name         string
		enabled      bool
		plates, x, y int
		want         string
	}{
		{"no_tower", false, 2, 0, 0, "not_applicable"},
		{"no_plate_metadata", true, 0, 0, 0, "plate_count_unknown"},
		{"partial_pairs", true, 2, 1, 2, "coordinates_incomplete"},
		{"no_pairs", true, 2, 0, 0, "coordinates_incomplete"},
		{"all_pairs_present", true, 2, 2, 2, "coordinates_present_clearance_unverified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			audit := towerPlatePositionAudit{plates: tc.plates, xEntries: tc.x, yEntries: tc.y}
			if got := audit.status(tc.enabled); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnknownPlateCountCallsOutUnverifiedTowerMetadata(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.3mf")
	output := filepath.Join(dir, "output.3mf")
	fixture(t, source, 4) // This is an editable 3MF, but model_settings has <config/> only.
	report, err := Convert(source, output, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if report["prime_tower_clearance_review_required"] != true ||
		report["prime_tower_plates"] != 0 ||
		report["prime_tower_plate_position_audit_status"] != "plate_count_unknown" ||
		report["prime_tower_plate_coordinate_review_required"] != false {
		t.Fatalf("unknown plate count must not become an approval: %+v", report)
	}
	warnings := strings.Join(report["warnings"].([]string), "\n")
	if !strings.Contains(warnings, "plate count could not be determined") {
		t.Fatalf("missing explicit unknown-plate QA warning: %s", warnings)
	}
}
