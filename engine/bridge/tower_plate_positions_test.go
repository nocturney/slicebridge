package bridge

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Two real Kobra3 plates with missing target tower X/Y failed OrcaSlicer
// 2.4.2 (192). One coordinate pair also failed; a separate pair per plate
// sliced in QA. Do not copy the QA (30,30) position into production.
func TestMultiPlateTowerCoordinateGapReportedWithoutAutofill(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.3mf")
	output := filepath.Join(dir, "target.3mf")
	fixture(t, source, 4)
	rewriteTestSourceArchive(t, source, func(name string, data []byte) []byte {
		if name == "Metadata/model_settings.config" {
			return []byte("<config><plate><metadata key='plater_id' value='1'/><model_instance><metadata key='object_id' value='1'/></model_instance></plate><plate><metadata key='plater_id' value='2'/><model_instance><metadata key='object_id' value='1'/></model_instance></plate></config>")
		}
		return data
	})
	report, err := Convert(source, output, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if report["prime_tower_plate_coordinate_review_required"] != true ||
		report["prime_tower_plates"] != 2 ||
		report["prime_tower_coordinate_x_entries"] != 1 ||
		report["prime_tower_coordinate_y_entries"] != 1 {
		t.Fatalf("missing per-plate coordinate evidence: %+v", report)
	}
	warnings := strings.Join(report["warnings"].([]string), "\n")
	if !strings.Contains(warnings, "2 plates") || !strings.Contains(warnings, "per-plate") {
		t.Fatalf("no actionable per-plate native slicer warning: %s", warnings)
	}
	cfg := map[string]any{}
	if err := json.Unmarshal([]byte(archiveContents(t, output)["Metadata/project_settings.config"]), &cfg); err != nil {
		t.Fatal(err)
	}
	// The built-in U1 target has one native tower position preset. The
	// converter must not fabricate a second pair for a two-plate project.
	x, xok := cfg["wipe_tower_x"].([]any)
	y, yok := cfg["wipe_tower_y"].([]any)
	if !xok || !yok || len(x) != 1 || len(y) != 1 {
		t.Fatalf("the converter modified/expanded target tower coordinates: X=%v Y=%v", cfg["wipe_tower_x"], cfg["wipe_tower_y"])
	}
}
