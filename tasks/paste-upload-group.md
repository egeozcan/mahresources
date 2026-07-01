# Task: Enable paste-to-upload on the group details page

## Request
"it should also be possible to upload by pasting to a group details page"

## Investigation findings
- Paste-to-upload already exists (`src/components/pasteUpload.js`), is wired in
  `src/main.js`, and the group detail template (`templates/displayGroup.tpl:7`)
  already carries `data-paste-context`. E2E tests (`26-paste-upload.spec.ts`)
  cover the bare group page and pass.
- Confirmed via a REAL clipboard paste in Chrome: on a *bare* group detail page
  the modal opens and a resource is created — so the happy path works.
- Root cause of the user's report: the global paste handler's **Guard 1** grabs
  the first `input[type='file']` on the page and merges the pasted file into it
  (legacy behaviour for the create-resource form). On a group detail page whose
  **category `CustomHeader`/`CustomSidebar` or a plugin slot renders any file
  input**, Guard 1 hijacks the paste — the upload modal never opens.
  - Reproduced with a real paste: `clipboardData.files.length === 1`, the file
    landed in the injected input, modal stayed closed.

## Change
- `src/components/pasteUpload.js`: Guard 1 now also requires
  `document.querySelector('[data-paste-context]') === null`. Pages built around a
  real upload form (createResource) carry no paste-context, so they keep the
  legacy "merge into the file input" behaviour. Detail / owner-filtered list
  pages carry a context, so the modal always wins.
- `e2e/tests/26-paste-upload.spec.ts`: new test #10 — pasting on a group detail
  page that also contains a file input opens the modal and does NOT let the file
  input swallow the paste.

## Review (verification)
- [x] Bug reproduced with a real paste on the old bundle (file hijacked, no modal).
- [x] Fix proven red->green via the new E2E test on the rebuilt bundle.
- [x] Legacy createResource path preserved (real paste lands in the file input, no modal).
- [x] All 15 paste-related E2E tests pass; Go unit tests pass.
- [x] Full E2E suite (browser + CLI) run.
- The change is logically a no-op for pages without `data-paste-context`
  (`!false && X === X`), so only context-bearing pages change behaviour.
