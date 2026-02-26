# Resource Details Metadata Redesign — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the flat key-value metadata table on the resource detail page with a card-based grid that surfaces primary fields and collapses technical details.

**Architecture:** Single template change in `displayResource.tpl`. Replace the `{% include "partials/json.tpl" %}` call with inline Pongo2 markup rendering cards directly from `resource` fields. Copy-to-clipboard uses the existing `updateClipboard()` from `src/index.js` via Alpine.js. No new JS files or CSS files.

**Tech Stack:** Pongo2 templates, Tailwind CSS, Alpine.js, existing `updateClipboard()` utility

---

### Task 1: Run existing tests to establish baseline

**Files:** None

**Step 1: Run Go unit tests**

Run: `go test ./...`
Expected: All tests pass (or note any pre-existing failures)

**Step 2: Build the application**

Run: `npm run build`
Expected: Build succeeds

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass (or note pre-existing failures). Pay attention to `08-resource.spec.ts` — the "should display the created resource" test checks for `h2:has-text("Resource")` which we are NOT changing.

**Step 4: Commit (nothing to commit — baseline only)**

No commit needed. This is verification only.

---

### Task 2: Replace metadata table with primary cards

**Files:**
- Modify: `templates/displayResource.tpl:12-14` (the json.tpl include block)

**Step 1: Replace the json.tpl include with card grid markup**

Replace lines 12-14 of `displayResource.tpl`:

```html
    <div class="mb-6">
        {% include "/partials/json.tpl" with jsonData=resource keys="ID,CreatedAt,UpdatedAt,Name,OriginalName,OriginalLocation,Hash,HashType,Location,StorageLocation,Description,Width,Height" %}
    </div>
```

With this card-based markup:

```html
    <section class="mb-6" aria-label="Resource metadata">
        <dl class="grid grid-cols-2 md:grid-cols-3 gap-3" x-data>
            {% if resource.Name %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Name</dt>
                <dd class="text-sm mt-0.5 break-all">{{ resource.Name }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Name"
                    @click="updateClipboard('{{ resource.Name|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            {% endif %}

            {% if resource.OriginalName %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Original Name</dt>
                <dd class="text-sm mt-0.5 break-all">{{ resource.OriginalName }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Original Name"
                    @click="updateClipboard('{{ resource.OriginalName|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            {% endif %}

            {% if resource.Width and resource.Height %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Dimensions</dt>
                <dd class="text-sm mt-0.5">{{ resource.Width }} × {{ resource.Height }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Dimensions"
                    @click="updateClipboard('{{ resource.Width }}x{{ resource.Height }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            {% endif %}

            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Created</dt>
                <dd class="text-sm mt-0.5">{{ resource.CreatedAt|date:"Jan 02, 2006 15:04" }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Created date"
                    @click="updateClipboard('{{ resource.CreatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>

            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Updated</dt>
                <dd class="text-sm mt-0.5">{{ resource.UpdatedAt|date:"Jan 02, 2006 15:04" }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Updated date"
                    @click="updateClipboard('{{ resource.UpdatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
        </dl>

        <details class="mt-3">
            <summary class="cursor-pointer text-sm text-gray-500 hover:text-gray-700 select-none py-1">Technical Details</summary>
            <dl class="grid grid-cols-2 md:grid-cols-3 gap-3 mt-3" x-data>
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">ID</dt>
                    <dd class="text-sm mt-0.5">{{ resource.ID }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy ID"
                        @click="updateClipboard('{{ resource.ID }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>

                {% if resource.Hash %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Hash{% if resource.HashType %} ({{ resource.HashType }}){% endif %}</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Hash }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Hash"
                        @click="updateClipboard('{{ resource.Hash }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}

                {% if resource.Location %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Location</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Location }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Location"
                        @click="updateClipboard('{{ resource.Location|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}

                {% if resource.OriginalLocation %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Original Location</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.OriginalLocation }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Original Location"
                        @click="updateClipboard('{{ resource.OriginalLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}

                {% if resource.StorageLocation %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Storage Location</dt>
                    <dd class="text-sm mt-0.5 break-all">{{ resource.StorageLocation }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Storage Location"
                        @click="updateClipboard('{{ resource.StorageLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}

                {% if resource.Description %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3 col-span-2 md:col-span-3">
                    <dt class="text-xs text-gray-500">Description</dt>
                    <dd class="text-sm mt-0.5">{{ resource.Description }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Description"
                        @click="updateClipboard('{{ resource.Description|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}
            </dl>
        </details>
    </section>
```

**Important notes:**
- `updateClipboard` is already exposed globally in `src/main.js` via `window.updateClipboard = updateClipboard`
- Verify this is true before proceeding. Check `src/main.js` for the global export. If it's not exported globally, the Alpine `@click` handlers won't work. In that case, either export it or use `navigator.clipboard.writeText()` directly.
- `|escapejs` is a Pongo2 filter that escapes strings for safe use in JavaScript string literals
- `|date` uses Go date format strings (the reference date is `Mon Jan 2 15:04:05 MST 2006`)
- The `×` character (×) is used for dimensions display instead of `x`
- `StorageLocation` is a `*string` (pointer) in Go — Pongo2 will render it correctly, and the `{% if %}` check handles nil

**Step 2: Verify the build still works**

Run: `npm run build`
Expected: Build succeeds (template changes don't affect the build, but confirm)

**Step 3: Commit**

```bash
git add templates/displayResource.tpl
git commit -m "feat: replace resource metadata table with card grid"
```

---

### Task 3: Verify updateClipboard is globally accessible

**Files:**
- Read: `src/main.js` (check for global export of `updateClipboard`)

**Step 1: Check if updateClipboard is on window**

Read `src/main.js` and search for `updateClipboard`. Look for either:
- `window.updateClipboard = updateClipboard` (direct export)
- It being passed to Alpine's `$store` or `Alpine.magic`

If NOT found globally, add this line to `src/main.js` after the import:

```js
window.updateClipboard = updateClipboard;
```

**Step 2: If changed, rebuild JS**

Run: `npm run build-js`
Expected: Build succeeds

**Step 3: Commit if changed**

```bash
git add src/main.js
git commit -m "feat: expose updateClipboard globally for Alpine template use"
```

---

### Task 4: Visual verification and E2E tests

**Files:** None

**Step 1: Build and start the server**

Run: `npm run build`
Expected: Build succeeds

**Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass. The key test is `08-resource.spec.ts` "should display the created resource" — it checks for `h2:has-text("Resource")` and `page.url` containing `/resource?id=`, neither of which are affected by our template change.

**Step 3: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y`
Expected: All a11y tests pass. Our new markup uses semantic `<dl>`/`<dt>`/`<dd>`, `aria-label` on buttons, and native `<details>`/`<summary>`.

**Step 4: Manual visual check (optional)**

Start the server in ephemeral mode and navigate to any resource detail page:

```bash
./mahresources -ephemeral -bind-address=:8181
```

Verify:
- Primary cards (Name, Original Name, Dimensions, Created, Updated) display in a grid
- "Technical Details" is collapsed by default
- Expanding it shows ID, Hash, Location, etc.
- Hover over a card shows the copy icon (⧉)
- Clicking the copy icon copies the value and shows ✓ briefly
- Empty fields are not shown
- Layout is responsive (2 cols on narrow, 3 on wider)

---

### Task 5: Final commit and cleanup

**Step 1: Verify no untracked files or missed changes**

Run: `git status`
Expected: Clean working tree (all changes committed in Tasks 2-3)

If there are uncommitted changes, commit them with an appropriate message.
