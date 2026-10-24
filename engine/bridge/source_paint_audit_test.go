package bridge

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BryanSmee/bl_to_std/pkg/paint"
)

// Only edits synthetic test fixtures; original user 3MF files are never touched.
func rewriteTestSourceArchive(t *testing.T, filename string, change func(string, []byte) []byte) {
	t.Helper()
	zr, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range zr.File {
		in, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(in)
		in.Close()
		if err != nil {
			t.Fatal(err)
		}
		data = change(file.Name, data)
		w, err := zw.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRealWorldPaintSlotsGuardRejectsOutOfPaletteSourceWithoutPublishing(t *testing.T) {
	// An original four-material 3MF can already have paint references to 5+
	// (and even 15) from a previous, larger color palette. Conversion cannot
	// truthfully present those orphan references as four reliably mapped tools.
	for _, want := range []struct {
		name     string
		paint    string
		expected string
	}{
		{"orphan_fifth_color", paint.Node{State: 5}.Encode(), "5"},
		{"orphan_nested_fifteenth_color", (paint.Node{
			SplitSides: 1, Children: []paint.Node{{State: 1}, {State: 15}},
		}).Encode(), "15"},
	} {
		t.Run(want.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.3mf")
			output := filepath.Join(dir, "converted.3mf")
			original := fixture(t, source, 4)
			rewriteTestSourceArchive(t, source, func(name string, data []byte) []byte {
				if name == "3D/3dmodel.model" {
					return bytes.Replace(data, []byte("paint_color='4'"),
						[]byte("paint_color='"+want.paint+"'"), 1)
				}
				return data
			})
			fixedSource, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(original, fixedSource) {
				t.Fatal("test failed to alter synthetic paint")
			}
			_, err = Convert(source, output, "u1")
			if err == nil || ErrorCode(err) != "SOURCE_PAINT_SLOT_UNRESOLVED" {
				t.Fatalf("expected precise unresolved source-paint refusal, got %v (code %s)", err, ErrorCode(err))
			}
			if !strings.Contains(err.Error(), want.expected) {
				t.Fatalf("missing actionable orphan ID %s: %v", want.expected, err)
			}
			requireNoOutput(t, dir, "source.3mf")
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(after, fixedSource) {
				t.Fatal("source was changed on refusal")
			}
		})
	}
}

func TestRealWorldPaintSlotsGuardRejectsOrphanObjectDefaultExtruder(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.3mf")
	output := filepath.Join(dir, "converted.3mf")
	fixture(t, source, 4)
	rewriteTestSourceArchive(t, source, func(name string, data []byte) []byte {
		if name == "Metadata/model_settings.config" {
			return []byte("<config><object id='1'><metadata key='extruder' value='1'/>" +
				"<part id='2' subtype='normal_part'><metadata key='extruder' value='7'/>" +
				"</part></object></config>")
		}
		return data
	})
	_, err := Convert(source, output, "u1")
	if err == nil || ErrorCode(err) != "SOURCE_PAINT_SLOT_UNRESOLVED" {
		t.Fatalf("out-of-palette default extruder cannot silently map to 1: %v", err)
	}
	if !strings.Contains(err.Error(), "7") {
		t.Fatalf("missing offending object default: %v", err)
	}
	requireNoOutput(t, dir, "source.3mf")
}

func TestRealWorldPaintSlotsGuardAcceptsFourthPaintedChannelAndUnpaintedLeaves(t *testing.T) {
	for _, tc := range []struct {
		name       string
		colorState int
	}{
		{"fourth_color", 4},
		{"unpainted", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.3mf")
			output := filepath.Join(dir, "converted.3mf")
			fixture(t, source, 4)
			target := paint.Node{State: tc.colorState}.Encode()
			rewriteTestSourceArchive(t, source, func(name string, data []byte) []byte {
				if name == "3D/3dmodel.model" {
					return bytes.Replace(data, []byte("paint_color='4'"),
						[]byte(fmt.Sprintf("paint_color='%s'", target)), 1)
				}
				return data
			})
			result, err := Convert(source, output, "u1")
			if err != nil {
				t.Fatal(err)
			}
			if result["source_filament_slots"] != 4 || result["tool_usage_verification_required"] != true {
				t.Fatalf("unexpected channel/safety report: %+v", result)
			}
			files := archiveContents(t, output)
			if !strings.Contains(files["3D/3dmodel.model"], "paint_color=\""+target+"\"") {
				t.Fatalf("valid painted filament changed or lost: %s", target)
			}
		})
	}
}

func TestRealWorldPaintSlotsGuardCoversPrusaMMUSegmentation(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.3mf")
	output := filepath.Join(dir, "converted.3mf")
	fixture(t, source, 4)
	orphan := paint.Node{State: 5}.Encode()
	rewriteTestSourceArchive(t, source, func(name string, data []byte) []byte {
		if name == "3D/3dmodel.model" {
			return bytes.Replace(data, []byte("paint_color='4'"),
				[]byte("mmu_segmentation='"+orphan+"'"), 1)
		}
		return data
	})
	_, err := Convert(source, output, "u1")
	if err == nil || ErrorCode(err) != "SOURCE_PAINT_SLOT_UNRESOLVED" {
		t.Fatalf("mmu_segmentation must be audited like paint_color: %v", err)
	}
	requireNoOutput(t, dir, "source.3mf")
}
