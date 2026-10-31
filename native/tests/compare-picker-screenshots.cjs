'use strict';
// Optional, offline visual regression check for screenshots from picker-layout-ci.cjs.
// Run the EXISTING browser/geometry QA first with PICKER_LAYOUT_CI_SCREENSHOTS set.
// Compare snapshots rendered on the same browser, OS, viewport, data and locale.
// Usage: node native/tests/compare-picker-screenshots.cjs \
//   --baseline /path/accepted --candidate /path/current --only en-ltr-1100x750.png \
//   --mode pixel --fail-under 99.9 --out-dir /path/diff-receipts
// No network, auth, printer, publishing or production action. No automatic CI gate.
const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const FLAGS = new Set(['--baseline', '--candidate', '--only', '--mode',
  '--fail-under', '--width', '--out-dir']);
function fail(message) {
  throw new Error(message);
}
function parse(argv) {
  const opts = {};
  for (let i = 0; i < argv.length; i++) {
    const key = argv[i];
    if (key === '--overwrite') {
      if (opts.overwrite) fail('duplicate --overwrite');
      opts.overwrite = true;
      continue;
    }
    if (!FLAGS.has(key)) fail('unknown argument ' + key);
    if (Object.hasOwn(opts, key)) fail('duplicate ' + key);
    if (i + 1 >= argv.length || argv[i + 1].startsWith('--'))
      fail('missing value for ' + key);
    opts[key] = argv[++i];
  }
  return opts;
}
function pngFiles(directory) {
  if (!fs.statSync(directory).isDirectory()) fail('not a directory: ' + directory);
  return fs.readdirSync(directory).filter(x => x.toLowerCase().endsWith('.png')).sort();
}
function main() {
  const opts = parse(process.argv.slice(2));
  if (!opts['--baseline'] || !opts['--candidate'])
    fail('required: --baseline <dir> --candidate <dir>');
  const baseline = path.resolve(opts['--baseline']);
  const candidate = path.resolve(opts['--candidate']);
  const mode = opts['--mode'] || 'pixel';
  if (!['pixel', 'layout'].includes(mode)) fail('--mode must be pixel or layout');
  const threshold = Number(opts['--fail-under'] || (mode === 'pixel' ? '99.9' : '90'));
  if (!Number.isFinite(threshold) || threshold < 0 || threshold > 100)
    fail('--fail-under must be between 0 and 100');
  const width = Number(opts['--width'] || '480');
  if (!Number.isInteger(width) || width < 64 || width > 2048)
    fail('--width must be an integer between 64 and 2048');
  const names = pngFiles(baseline);
  const candidates = pngFiles(candidate);
  let selected = names;
  if (opts['--only']) {
    const only = opts['--only'];
    if (only !== path.basename(only) || !only.toLowerCase().endsWith('.png'))
      fail('--only must be a PNG filename, not a path');
    selected = [only];
  } else if (JSON.stringify(names) !== JSON.stringify(candidates)) {
    fail('baseline and candidate screenshot filenames differ');
  }
  if (!selected.length) fail('no screenshots selected');
  for (const name of selected) {
    if (!names.includes(name) || !candidates.includes(name))
      fail('missing matched screenshot: ' + name);
  }
  const outDir = opts['--out-dir'] ? path.resolve(opts['--out-dir']) : null;
  if (outDir) {
    // Refuse accidental overwrites of evidence.
    const present = selected.filter(name => fs.existsSync(path.join(outDir, name)));
    if (present.length && !opts.overwrite)
      fail('diff evidence already exists, use --overwrite explicitly: ' + present.join(', '));
    fs.mkdirSync(outDir, {recursive: true});
  }
  const python = process.env.PYTHON3 || 'python3';
  const tool = path.join(__dirname, 'vendor', 'replica-imgdiff', 'imgdiff.py');
  if (!fs.existsSync(tool)) fail('missing pinned local diff tool');
  let failed = 0;
  for (const name of selected) {
    const args = [tool, path.join(baseline, name), path.join(candidate, name),
      '--mode', mode, '--width', String(width), '--fail-under', String(threshold), '--json'];
    if (outDir) args.push('--out', path.join(outDir, name));
    const result = spawnSync(python, args, {encoding:'utf8', timeout:120000,
      maxBuffer:8 * 1024 * 1024, env:{...process.env, PYTHONDONTWRITEBYTECODE:'1'}});
    if (result.error || ![0, 1].includes(result.status)) {
      failed++;
      console.error('VISUAL_DIFF ERROR ' + name + ': ' +
        (result.error?.message || result.stderr || 'invalid tool exit ' + result.status));
      continue;
    }
    let report;
    try { report = JSON.parse(result.stdout); }
    catch (_) {
      failed++;
      console.error('VISUAL_DIFF ERROR ' + name + ': invalid JSON result');
      continue;
    }
    const sameDimensions = report.original?.width === report.clone?.width &&
      report.original?.height === report.clone?.height;
    const pass = result.status === 0 && sameDimensions;
    if (!pass) failed++;
    console.log(JSON.stringify({
      screenshot:name, result:pass ? 'PASS' : 'FAIL',
      mode, score:report.score, threshold, changed_pixels:report.changed_pixels ?? null,
      regions:report.regions?.length ?? null, same_dimensions:sameDimensions,
      diff_image:outDir ? path.join(outDir, name) : null,
      reason:!sameDimensions ? 'viewport mismatch' : result.status === 1 ? 'under threshold' : null
    }));
  }
  console.log('VISUAL_DIFF ' + (failed ? 'FAIL' : 'PASS') +
    ': compared=' + selected.length + ' failed=' + failed);
  if (failed) process.exitCode = 1;
}
try { main(); }
catch (error) {
  console.error('VISUAL_DIFF ERROR: ' + error.message);
  process.exitCode = 2;
}
