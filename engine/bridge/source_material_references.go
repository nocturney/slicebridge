package bridge

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/BryanSmee/bl_to_std/pkg/converter"
	"github.com/BryanSmee/bl_to_std/pkg/paint"
)

// validateSourceMaterialReferences enforces a relationship the converted 3MF
// must preserve: every *painted filament* and every explicit positive object
// default extruder must refer to a declared source filament ID.
//
// Some externally authored projects carry stale paint IDs from a larger
// palette. The 3MF can look structurally correct while the target slicer
// silently omits colors; remapping missing IDs to color 1 would invent a print
// assignment. Reject such sources BEFORE creating any output, leaving both
// source and installed OEM/user profiles unchanged.
func validateSourceMaterialReferences(source string, filaments []converter.Filament) error {
	declared := make(map[int]bool, len(filaments))
	for _, f := range filaments {
		declared[f.ID] = true
	}
	archive, err := zip.OpenReader(source)
	if err != nil {
		return fail("SOURCE_INVALID", "cannot inspect source paint references: %v", err)
	}
	defer archive.Close()

	unresolvedPaint := map[int]bool{}
	unresolvedDefaults := map[int]bool{}
	cache := map[string][]int{}
	for _, f := range archive.File {
		model := strings.HasSuffix(strings.ToLower(f.Name), ".model")
		metadata := f.Name == "Metadata/model_settings.config"
		if !model && !metadata {
			continue
		}
		if err := auditMaterialReferenceEntry(f, model, declared, cache, unresolvedPaint, unresolvedDefaults); err != nil {
			return fail("SOURCE_INVALID", "cannot inspect paint reference %s: %v", f.Name, err)
		}
	}
	if len(unresolvedPaint) != 0 || len(unresolvedDefaults) != 0 {
		return fail("SOURCE_PAINT_SLOT_UNRESOLVED",
			"source 3MF contains filament references absent from its declared palette: painted IDs %v; object/part default extruder IDs %v; declared IDs %v. Reassign these colors/materials in the original slicer and export a consistent 3MF; SliceBridge will not guess a physical-tool mapping.",
			sortedMaterialIDs(unresolvedPaint), sortedMaterialIDs(unresolvedDefaults), sortedMaterialIDs(declared))
	}
	return nil
}

func auditMaterialReferenceEntry(f *zip.File, model bool, declared map[int]bool, cache map[string][]int, missingPaint, missingDefaults map[int]bool) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	decoder := xml.NewDecoder(rc)
	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch {
		case model && start.Name.Local == "triangle":
			for _, attr := range start.Attr {
				// The pinned converter interprets both formats as the
				// same 1-based filament IDs (Bambu and Prusa respectively).
				if (attr.Name.Local != "paint_color" && attr.Name.Local != "mmu_segmentation") || attr.Value == "" {
					continue
				}
				refs, seen := cache[attr.Value]
				if !seen {
					node, err := paint.Decode(attr.Value)
					if err != nil {
						return fmt.Errorf("invalid painted triangle state: %w", err)
					}
					for filament := range node.Filaments() {
						refs = append(refs, filament)
					}
					sort.Ints(refs)
					cache[attr.Value] = refs
				}
				for _, filament := range refs {
					if !declared[filament] {
						missingPaint[filament] = true
					}
				}
			}
		case !model && start.Name.Local == "metadata":
			var key, value string
			for _, attr := range start.Attr {
				if attr.Name.Local == "key" {
					key = attr.Value
				} else if attr.Name.Local == "value" {
					value = attr.Value
				}
			}
			if key != "extruder" {
				continue
			}
			filament, err := strconv.Atoi(value)
			if err != nil || filament <= 0 {
				// Zero can mean inherited/default selection; unfamiliar
				// non-numeric OEM values are left to the existing converter.
				continue
			}
			if !declared[filament] {
				missingDefaults[filament] = true
			}
		}
	}
}

func sortedMaterialIDs(m map[int]bool) []int {
	result := make([]int, 0, len(m))
	for id := range m {
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}
