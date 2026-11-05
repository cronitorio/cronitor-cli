const assert = require('node:assert/strict');
const { test } = require('node:test');
const path = require('node:path');
const lock = require('../package-lock.json');

const root = path.resolve(__dirname, '..');
const packages = name => Object.entries(lock.packages)
  .filter(([location]) => location.endsWith(`node_modules/${name}`));

function atLeast(version, minimum) {
  const actual = version.split('.').map(Number);
  return actual[0] === minimum[0] &&
    (actual[1] > minimum[1] || (actual[1] === minimum[1] && actual[2] >= minimum[2]));
}

test('both js-yaml major versions are patched and retain their callers APIs', () => {
  const copies = packages('js-yaml');
  assert.ok(copies.length > 0);
  for (const [location, metadata] of copies) {
    const major = Number(metadata.version.split('.')[0]);
    const minimum = major === 3 ? [3, 15, 2] : [4, 3, 2];
    assert.ok(atLeast(metadata.version, minimum), `${location}: ${metadata.version}`);
    const yaml = require(path.join(root, location));
    const parse = major === 3 ? yaml.safeLoad : yaml.load;
    assert.deepEqual(parse('defaults: &defaults\n  enabled: true\njob:\n  <<: *defaults\n'), {
      defaults: { enabled: true }, job: { enabled: true },
    });
  }
});

test('SVGO is patched and still optimizes SVGs', () => {
  const copies = packages('svgo');
  assert.ok(copies.length > 0);
  for (const [location, metadata] of copies) {
    assert.ok(atLeast(metadata.version, [3, 3, 5]), `${location}: ${metadata.version}`);
    const { optimize } = require(path.join(root, location));
    const result = optimize('<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>');
    assert.match(result.data, /<svg/);
  }
});

test('underscore is patched and still flattens, compares, and uniq-s values', () => {
  const copies = packages('underscore');
  assert.ok(copies.length > 0);
  for (const [location, metadata] of copies) {
    assert.ok(atLeast(metadata.version, [1, 13, 8]), `${location}: ${metadata.version}`);
    const _ = require(path.join(root, location));
    assert.deepEqual(_.flatten([1, [2, [3]]]), [1, 2, 3]);
    assert.deepEqual(_.flatten([1, [2, [3]]], true), [1, 2, [3]]);
    assert.equal(_.isEqual({ a: 1 }, { a: 1 }), true);
    assert.deepEqual(_.uniq([1, 2, 1, 3, 2]), [1, 2, 3]);
  }
});

test('serialize-javascript is patched and still round-trips objects', () => {
  const copies = packages('serialize-javascript');
  assert.ok(copies.length > 0);
  for (const [location, metadata] of copies) {
    assert.ok(atLeast(metadata.version, [7, 1, 3]), `${location}: ${metadata.version}`);
    const serialize = require(path.join(root, location));
    const restored = eval('(' + serialize({
      ok: true,
      re: /cron/i,
      fn() { return 1; },
    }) + ')');
    assert.equal(restored.ok, true);
    assert.equal(restored.re.source, 'cron');
    assert.equal(restored.re.flags, 'i');
    assert.equal(restored.fn(), 1);
  }
});

test('Puppeteer no longer brings in extract-zip', async () => {
  assert.equal(packages('extract-zip').length, 0);
  const { default: puppeteer } = await import('puppeteer');
  assert.equal(typeof puppeteer.launch, 'function');
  const { captureScreenshots } = require('./capture-screenshots');
  assert.equal(typeof captureScreenshots, 'function');
});

test('Puppeteer can render a page and capture a screenshot', {
  skip: !process.env.PUPPETEER_EXECUTABLE_PATH,
}, async () => {
  const { default: puppeteer } = await import('puppeteer');
  const browser = await puppeteer.launch({
    executablePath: process.env.PUPPETEER_EXECUTABLE_PATH,
    args: process.env.PUPPETEER_TEST_NO_SANDBOX === '1' ? ['--no-sandbox'] : [],
  });
  try {
    const page = await browser.newPage();
    await page.setViewport({ width: 1440, height: 900 });
    await page.setContent('<h1>Screenshot smoke test</h1>');
    assert.equal(await page.$eval('h1', el => el.textContent), 'Screenshot smoke test');
    const png = await page.screenshot();
    assert.ok(png.length > 100);
  } finally {
    await browser.close();
  }
});
