# SliceBridge controlled visual-diff pilot — 2026-10-28

**Source:** canonical private velvet-workspace/slicebridge main commit 03ea82a0a9a08667d90b59526ce0b1cf9b9f7f91.
**Renderer:** Microsoft Edge headless on authorized MacMiniOffice.local.
**Fixture:** existing native/tests/picker-layout-ci.cjs with real SliceBridge HTML/CSS/JS and synthetic offline printer data, en-ltr-1100x750. Screenshot host: 1200x1000. This is **not** an observed production bug.

## Controlled screenshots

- [Baseline browser screenshot](baseline.png): real product frontend, no synthetic CSS change.
- [Synthetic shifted heading](synthetic-header-shift.png): same source/data, with one temporary **uncommitted** CSS test mutation.
- [Pixel diff](pixel-diff.png): marks altered pixels red near the top.
- CSS mutation: .my-printers-dialog #myPrintersTitle,.my-printers-dialog #myPrintersIntro { transform: translateX(38px) !important; }
- The original UI geometry assertion returned PASS for both versions: the title position is not measured.

## Results on identical fixture

| Check | Clean replay | Heading shift |
| --- | ---: | ---: |
| Microsoft Edge geometry test | PASS | PASS |
| Screenshot bytes / SHA-256 | Identical | Different |
| Replica pixel score | 100.0 | 99.6 |
| Changed pixels at 480px comparison width | 0 | 702 |
| Diff regions | 0 | 1 |
| Explicit fail-under 99.9 | PASS / exit 0 | FAIL / exit 1 |

Two clean captures on updated main had SHA-256 17c379040ba54efa888565e301660067193000a5edd3a131e72078c5334aae08. The full existing layout suite passed **11/11** on the previous c5af89838a024b24f28bd37f77499ab8487c6c29 commit; the focused fixture passed baseline, mutated and replay runs at current 03ea82a0a9a08667d90b59526ce0b1cf9b9f7f91.

## Implementation and limitations

- Optional comparator: native/tests/compare-picker-screenshots.cjs, invoked directly from ChatGPT through the existing authorized desktop connection.
- Local-only fast scenario: PICKER_LAYOUT_CI_ONLY=en-ltr-1100x750. All 11 remain the default, and the filter is forbidden when CI is set.
- Unit/regression tests: node --test native/tests/compare-picker-screenshots.test.js.
- Pinned MIT diff tool: Jakeschincariol/replica-skill commit 77c9436fb3d18c3d58169efb8caf4fe906b0dc51. License retained in native/tests/vendor/replica-imgdiff/LICENSE.
- Upstream default layout mode scored this shift 98.6 / matches with no regions, so use explicitly selected pixel threshold and inspect the image. The strict pixel threshold has only been validated on one browser/OS/fixture. Do not enable it as a global CI or release gate without multi-renderer calibration.
- No new skill/registry, Claude dependency, paid API, production change, merge, deployment, auth or payment action.
