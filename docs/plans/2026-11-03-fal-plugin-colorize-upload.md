# Fix fal.ai Colorize upload failures

Success: Colorize prepares and submits an image whose data URI exceeds one
million bytes, preserving the uploaded bytes and MIME type, without raising
`pattern/input too complex`. Both DDColor and Topaz use the shared upload path.

The deployed server is on master `63ca1ea6`; the original local checkout was
behind it. Work starts from that deployed revision. The failure occurs before
model submission, in the Lua pattern that captures the whole base64 payload.

## Plan

- [x] Read the live failure and match the deployed source revision.
- [x] Reproduce the exact failure in the existing mocked CDN action harness.
- [x] Split the URI at its literal comma and validate only the short header.
  Preserve invalid-base64 handling and the inline fallback on upload failure.
- [x] Verify both Colorize models, large input and binary byte preservation,
  then run the relevant plugin suite and broader checks where available.
- [x] Review the diff and record verification and deployment status.

The change stays in the shared Lua upload parser; it introduces no host API,
dependency, model changes, image recompression or size restriction. Model and
CDN calls in regression tests are mocked to avoid paid requests.

## Review

The regression reproduced `plugin.lua:283: pattern/input too complex` in all
four DDColor/Topaz upload/fallback cases before the fix. They pass afterwards,
including exact binary bytes and MIME preservation. Invalid base64 still falls
back inline without uploading. All Fal tests pass.

Browser and CLI plugin checks passed together against ephemeral servers:
38 tests. PostgreSQL checks could not start because no Docker provider is
available. The full plugin suite passes with the repository's declared
Go 1.26.4 toolchain (157 seconds). The broader Go 1.27.2 run passed every
package except the plugin suite, where that newer toolchain changes a
JSON-depth error string expected by a pre-existing streaming test. Broader
application/server/architecture checks pass with Go 1.26.4. The focused
large-input regression also passes with the race detector.

Applied the identical tested Lua file on mahlayf after verifying its original
SHA256 matched master. Kept the original at
`/tmp/mah-fal-ai-plugin-before-2026-11-03.lua`. Checked that fal-ai had no active
Jobs and reloaded only that plugin using its normal disable/enable endpoints;
both calls succeeded. The user then clarified that mahlayf deploys on push.
Removed only the verified server-side edit and confirmed the tracked working
tree is clean; existing untracked files were left untouched. The fixed plugin
remains loaded in memory until its next reload or process restart. Delivery
follows the normal push-to-master workflow on both origin and mahlayf, as
requested by the user. No paid model request has been made during verification.
