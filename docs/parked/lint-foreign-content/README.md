# Parked: foreign-content and `<noscript>` handling in the bare-value placement linter

Parked 2026-08-23, after ten adversarial review rounds. The work is real and the tests pass;
it is parked because the review gate was never met and round 10 showed the change still has
missed-warning defects. Nothing here is speculative: every claim below has a reproduction.

**Branch:** `wip/lint-foreign-content`, 16 commits, based on master `9c318059`.
**Diff size:** +2193 / -117, of which 1125 added lines are in `shortcodes/lint_attr_context.go`.
**Tests:** `go test --tags 'json1 fts5' ./...` passes, exit 0, 37 packages, 0 failures.
**Gate:** two consecutive pi rounds with zero MAJOR findings. **Not met.** Ten rounds ran and
every one produced majors.

## Read this before trusting anything in the branch

`shortcodes/lint_attr_context.go:1566-1574` claims the region-recursion residue fails only in
the safe direction, warning too much rather than too little. **That comment is wrong.** Round
10 findings 3 and 4 each produce a live `srcdoc` the linter is silent on. Anyone resuming this
who trusts that comment starts from a false premise.

This is branch-only. The branch is unmerged, so no incorrect comment reached master.

## What the change does

The linter warns when `[meta inline]`, `[property]`, `[item]` or `[mrql value=]` lands where
HTML-escaping does not protect it. Two pre-existing gaps were the target:

1. **SVG/MathML foreign content.** `golang.org/x/net/html`'s tokenizer is namespace-unaware and
   raw-texts `script`, `iframe` and friends by element name wherever they appear. Inside `<svg>`
   or `<math>` those are foreign content, where entities are decoded and `<iframe>` is not raw
   text. Consequences: the `<svg><script>` message stated the opposite of the truth, and
   `<svg><iframe><a href="javascript:...">` was silent while being a live SVG link.
2. **`<noscript>`.** With scripting off the body is real markup. Only the non-execution set is
   reportable there (srcdoc, unquoted attribute, `style=`, interpolated attribute name, `raw=`,
   and a `<style>` element). Every execution rule must stay withheld, because a `javascript:` or
   `on*` warning inside `<noscript>` is a false positive under both scripting modes.

An earlier assessment held that gap 1 needed a parser rewrite. That was tested with
`html.ParseWithOptions` and disproved: the implementation stays on the tokenizer, which
preserves the byte offsets every diagnostic is anchored to, and re-reads the swallowed region
through a recursive depth-bounded `scanMarkup`.

Scope grew past those two. A parser-differential oracle (`TestLintPlacementsAgainstTheParser`)
surfaced large pre-existing false-positive classes and the change removes them: 795 to 0 on
srcdoc, 272 to 0 on program bodies.

## The immediate to-do: round 10's five majors

Full text in `laneA2-pi-r10.txt`. Each carries a reproduction checked against `html.Parse`.
Four are missed warnings, one is a false positive.

1. **Missed.** The outer tokenizer truncates foreign CDATA at a fake raw-text end tag before
   recursion. `<svg><script><![CDATA[var x="</script>"; var y="[property path='Name']";]]></script></svg>`
   reaches JavaScript with no warning.
2. **False positive.** `AllowCDATA` uses the child namespace at an SVG `<title>` integration
   point instead of the current element's, so a CDATA payload is read as a live `srcdoc` that
   `html.Parse` never creates.
3. **Missed.** Caller end-tag effects apply only after the recursive region's remainder has been
   scanned in the stale namespace.
4. **Missed.** Only the last unmatched end tag is forwarded, so a later no-op tag erases an
   earlier namespace-changing one.
5. **Missed.** Generic stack popping does not model the adoption agency algorithm, so `<b>`,
   `<a>` or `<nobr>` before a `<div>` loses the later close.

Findings 3 and 4 are the ones that disprove the comment quoted above.

## Review history

Ten captures are in this directory: `laneA-pi-r1.txt` through `laneA-pi-r9.txt` and
`laneA2-pi-r10.txt`. They are the record of what has already been tried and refuted. Resuming
without reading them repeats the loop.

Rounds 1 to 9 ran without a severity rubric in the reviewer's prompt, which is part of why the
gate never closed: the agent enforcing the gate knew what "major" meant and the reviewer
producing the verdicts did not. Round 10 onward carries an explicit rubric, every finding
tagged, and a mandatory `MAJOR COUNT: N` tally line.

The rubric was backwards-calibrated against rounds 1 to 9 before being accepted: roughly 24 of
29 findings, including every one that led to a code fix, still score MAJOR under it. One
genuine downgrade was found and corrected by adding clause (f): an oracle gap that would mask a
real defect is MAJOR, while a bare coverage suggestion stays MINOR.

## Known residue, all documented in-file

- `<svg><g><title>Icon</g><iframe srcdoc=...>`: a re-read region reads its remainder in the
  namespace it started in. Needs the caller's open elements carried into the recursion.
- `<script type="application/ld+json">` warns "reaches JavaScript". Pre-existing, needs MIME
  classification. `<style>` did get the check.
- URL rules are element-blind (`<div href>`, `<svg><g href>`), as are `on*` prefix matches.
- Nested `<form>` action= needs the form element pointer.
- `<plaintext>` raw=; comments and the `-->` breakout; inName and unterminated inside
  `<noscript>` lacking the scripting-mode caveat; depth-cap silence past 8 levels.

## Recommendation

Split rather than resume as-is. The small verifiable core is the corrected `<svg><script>`
message and the `<noscript>` non-execution set. The foreign-content scan machinery is what
round 10 is still finding defects in, and 1125 added lines to a security-relevant scanner
earned ten rounds of majors for a reason.

One precedent worth weighing: earlier in the same campaign a lane withdrew a change to this
same file entirely, after proving with primary sources that the premise it had been given was
false and that both attempted fixes warned where nothing could go wrong. A warning that fires
on safe markup is worse than silence, because authors learn to ignore the linter.
