# Pinned Replica visual diff (test-only)

- Upstream: https://github.com/Jakeschincariol/replica-skill
- Source file: `replica-diff/imgdiff.py`
- Exact upstream commit: `77c9436fb3d18c3d58169efb8caf4fe906b0dc51`
- Pinned source blob: `74311b31472959ad81d5b62f951db83a54721aec`
- License: MIT; the unchanged upstream license is retained in `LICENSE`.
- Scope: isolated offline screenshot comparison from the existing SliceBridge UI fixture; no other Replica skills, Claude runtime, provider access or deployment path.
- Integration: `../../compare-picker-screenshots.cjs` via explicit opt-in CLI, not an automatic release/CI gate.

Recheck upstream license, tests and screenshot algorithm before any future update. Do not treat the raw score/verdict as release authority; same renderer, viewport, state and data are mandatory for pixel regression thresholds.
