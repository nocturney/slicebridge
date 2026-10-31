'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const zlib = require('node:zlib');
const {spawnSync} = require('node:child_process');
const entry = path.join(__dirname, 'compare-picker-screenshots.cjs');

function crc32(buffer) {
  let crc = 0xffffffff;
  for (const value of buffer) {
    crc ^= value;
    for (let bit = 0; bit < 8; bit++)
      crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0);
  }
  return (crc ^ 0xffffffff) >>> 0;
}
function chunk(type, data) {
  const label = Buffer.from(type, 'ascii');
  const size = Buffer.alloc(4);
  size.writeUInt32BE(data.length);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(Buffer.concat([label, data])));
  return Buffer.concat([size, label, data, crc]);
}
function screenshot(filename, xOffset = 0) {
  const width = 100, height = 60;
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8; header[9] = 2;  // RGB8, no alpha
  const bytes = Buffer.alloc(height * (1 + width * 3));
  for (let y = 0; y < height; y++) {
    const offset = y * (1 + width * 3);
    for (let x = 0; x < width; x++) {
      const black = x >= 14 + xOffset && x < 56 + xOffset && y >= 20 && y < 38;
      bytes.fill(black ? 0 : 255, offset + 1 + x * 3, offset + 4 + x * 3);
    }
  }
  fs.writeFileSync(filename, Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk('IHDR', header), chunk('IDAT', zlib.deflateSync(bytes)),
    chunk('IEND', Buffer.alloc(0))
  ]));
}
function fixture(t) {
  const base = fs.mkdtempSync(path.join(os.tmpdir(), 'slice-visual-'));
  t.after(() => fs.rmSync(base, {recursive:true, force:true}));
  const baseline = path.join(base, 'reference');
  const candidate = path.join(base, 'candidate');
  fs.mkdirSync(baseline); fs.mkdirSync(candidate);
  const name = 'screen.png';
  screenshot(path.join(baseline, name));
  screenshot(path.join(candidate, name));
  return {baseline, candidate, name};
}
function run(fixture) {
  return spawnSync(process.execPath, [entry, '--baseline', fixture.baseline,
    '--candidate', fixture.candidate, '--only', fixture.name,
    '--mode', 'pixel', '--fail-under', '99.9', '--width', '120'], {
      encoding:'utf8', timeout:60000
    });
}
test('identical PNG screenshots pass the explicit visual gate', t => {
  const f = fixture(t);
  const result = run(f);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.match(result.stdout, /"score":100/);
  assert.match(result.stdout, /VISUAL_DIFF PASS/);
});
test('controlled moved element fails despite unchanged image dimensions', t => {
  const f = fixture(t);
  screenshot(path.join(f.candidate, f.name), 22);
  const result = run(f);
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stdout, /"same_dimensions":true/);
  assert.match(result.stdout, /VISUAL_DIFF FAIL/);
});
test('missing paired screenshot fails closed', t => {
  const f = fixture(t);
  fs.unlinkSync(path.join(f.candidate, f.name));
  const result = run(f);
  assert.equal(result.status, 2, result.stdout + result.stderr);
  assert.match(result.stderr, /missing matched screenshot/);
});
