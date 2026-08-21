#!/usr/bin/env node
/**
 * Turn Playwright's JSON report into GitHub `::warning::` annotations for every
 * test that only passed on retry.
 *
 * Why this exists: CI runs the browser projects with `--retries=2`, and uploads
 * the report only `if: failure()`. A genuine intermittent regression that passes
 * on the second attempt therefore leaves CI green with no artifact and no trace
 * anywhere in the log -- the run is indistinguishable from one that passed first
 * time. This makes the flake visible without making it fatal.
 *
 * Deliberately NOT `--fail-on-flaky-tests`: this suite has a documented
 * load-induced flake class (`page.goto` timeouts under parallel workers), and
 * turning those red would trade an invisible signal for a false one.
 *
 * The step that runs this must never fail the job, so every failure mode here --
 * absent report, truncated JSON, a reporter shape this does not recognise --
 * exits 0. A missing signal is the status quo; a broken annotation step that
 * reds a green run is worse than the finding it closes.
 *
 * Usage: node scripts/report-flakes.js [path/to/results.json] [label]
 */

const fs = require('fs');
const path = require('path');

const reportPath = process.argv[2] || path.join('test-results', 'results.json');
const label = process.argv[3] || '';

// A GitHub `::warning file=...::` path is resolved against the repository root,
// but Playwright reports `spec.file` relative to `config.rootDir` -- which is
// `e2e/tests` here, and this script runs with `e2e` as its working directory.
// Annotating the reporter's value verbatim points at `cli/foo.spec.ts`, which
// exists nowhere in the repo, so GitHub silently drops the link.
const repoRoot = path.resolve(process.env.GITHUB_WORKSPACE || path.join(__dirname, '..', '..'));

/**
 * Absolute directory that `spec.file` is relative to. `config.rootDir` is the
 * reporter's own answer and is preferred; when a report shape omits it, it is
 * derived from where the report itself sits (`e2e/test-results/results.json` ->
 * `e2e/tests`) rather than from a prefix hardcoded here.
 */
function resolveSpecRoot(report) {
  const fromConfig = report && report.config && report.config.rootDir;
  if (typeof fromConfig === 'string' && fromConfig) {
    return path.resolve(fromConfig);
  }
  return path.resolve(path.dirname(path.resolve(reportPath)), '..', 'tests');
}

/**
 * Rewrite one reported spec path to repository-root-relative, in POSIX form.
 * Anything it cannot place inside the repo -- an absolute path from another
 * checkout, a rootDir that does not match this tree -- falls back to the raw
 * value: a wrong link is no worse than the one this replaces, and an exception
 * here would cost the whole annotation.
 */
function repoRelative(file, specRoot) {
  const raw = String(file || '');
  if (!raw) return '';
  try {
    const abs = path.isAbsolute(raw) ? raw : path.resolve(specRoot, raw);
    const rel = path.relative(repoRoot, abs);
    if (!rel || rel.startsWith('..') || path.isAbsolute(rel)) return raw;
    return rel.split(path.sep).join('/');
  } catch (err) {
    return raw;
  }
}

/** GitHub's annotation format is line-oriented; these three characters break it. */
function escapeAnnotation(s) {
  return String(s).replace(/%/g, '%25').replace(/\r/g, '%0D').replace(/\n/g, '%0A');
}

/**
 * Walk the nested `suites` tree and yield every spec whose test only passed on
 * retry. Playwright marks that two ways -- `test.status === 'flaky'`, and a
 * results array whose last entry passed after an earlier one did not -- and the
 * second is checked too so a reporter version that omits the status still
 * reports something.
 */
function collectFlaky(suites, trail, out) {
  for (const suite of suites || []) {
    const here = suite.title ? trail.concat(suite.title) : trail;
    for (const spec of suite.specs || []) {
      for (const test of spec.tests || []) {
        const results = test.results || [];
        const retried = results.length > 1 && results[results.length - 1].status === 'passed';
        if (test.status === 'flaky' || (retried && results.some((r) => r.status !== 'passed'))) {
          out.push({
            file: spec.file || suite.file || '',
            line: spec.line,
            title: here.concat(spec.title || '').filter(Boolean).join(' > '),
            attempts: results.length,
          });
          break;
        }
      }
    }
    collectFlaky(suite.suites, here, out);
  }
}

function main() {
  if (!fs.existsSync(reportPath)) {
    console.log(`report-flakes: no report at ${reportPath}; nothing to check.`);
    return false;
  }

  let report;
  try {
    report = JSON.parse(fs.readFileSync(reportPath, 'utf-8'));
  } catch (err) {
    console.log(`report-flakes: could not parse ${reportPath} (${err.message}); skipping.`);
    return false;
  }

  const flaky = [];
  try {
    collectFlaky(report.suites, [], flaky);
  } catch (err) {
    console.log(`report-flakes: unexpected report shape (${err.message}); skipping.`);
  }

  // stats.flaky is the reporter's own count. It is used as a backstop so a run
  // whose per-spec walk found nothing still says so when the summary disagrees.
  const reported = Number(report?.stats?.flaky) || 0;

  if (flaky.length === 0 && reported === 0) {
    console.log('report-flakes: no flaky tests.');
    return false;
  }

  const where = label ? ` [${label}]` : '';
  const specRoot = resolveSpecRoot(report);
  for (const f of flaky) {
    const file = repoRelative(f.file, specRoot);
    const loc = [file ? `file=${file}` : '', f.line ? `line=${f.line}` : '']
      .filter(Boolean)
      .join(',');
    const head = loc ? `::warning ${loc},title=Flaky test::` : '::warning title=Flaky test::';
    console.log(
      head +
        escapeAnnotation(
          `Flaky${where}: "${f.title}" passed only on retry (${f.attempts} attempts). ` +
            'It is not failing CI, but it is a real intermittent -- see the uploaded Playwright report.'
        )
    );
  }

  if (flaky.length === 0 && reported > 0) {
    console.log(
      `::warning title=Flaky test::${escapeAnnotation(
        `Playwright reported ${reported} flaky test(s)${where} but none could be named from the JSON report.`
      )}`
    );
  }

  console.log(`report-flakes: ${Math.max(flaky.length, reported)} flaky test(s)${where}.`);
  return true;
}

let anyFlaky = false;
try {
  anyFlaky = main();
} catch (err) {
  // Belt and braces: this step is advisory, so nothing it does may red the job.
  console.log(`report-flakes: skipped (${err && err.message});`);
}

// Consumed by the artifact-upload step's `if:` so a flaky-but-green run still
// keeps its report.
if (process.env.GITHUB_OUTPUT) {
  try {
    fs.appendFileSync(process.env.GITHUB_OUTPUT, `flaky=${anyFlaky ? 'true' : 'false'}\n`);
  } catch (err) {
    console.log(`report-flakes: could not write GITHUB_OUTPUT (${err.message}).`);
  }
}

process.exit(0);
