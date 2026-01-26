# Version Compare UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a dedicated comparison page for comparing resource versions visually with image diff modes, text diff, PDF viewing, and metadata comparison.

**Architecture:** A new template page at `/resource/compare` renders comparison UI. Cross-resource comparison extends the existing `CompareVersions` backend. Alpine.js components handle image comparison modes and text diffing client-side.

**Tech Stack:** Go/pongo2 templates, Alpine.js, jsdiff library for text diffing, Tailwind CSS

---

## Task 1: Extend Backend for Cross-Resource Comparison

**Files:**
- Modify: `application_context/resource_version_context.go:496-532`
- Modify: `models/query_models/version_query.go`

**Step 1: Add cross-resource comparison query model**

In `models/query_models/version_query.go`, add:

```go
// CrossVersionCompareQuery for comparing versions across different resources
type CrossVersionCompareQuery struct {
	Resource1ID uint `schema:"r1"`
	Version1    int  `schema:"v1"`
	Resource2ID uint `schema:"r2"`
	Version2    int  `schema:"v2"`
}
```

**Step 2: Extend VersionComparison struct**

In `application_context/resource_version_context.go`, update:

```go
// VersionComparison holds comparison data between two versions
type VersionComparison struct {
	Version1       *models.ResourceVersion `json:"version1"`
	Version2       *models.ResourceVersion `json:"version2"`
	Resource1      *models.Resource        `json:"resource1,omitempty"`
	Resource2      *models.Resource        `json:"resource2,omitempty"`
	SizeDelta      int64                   `json:"sizeDelta"`
	SameHash       bool                    `json:"sameHash"`
	SameType       bool                    `json:"sameType"`
	DimensionsDiff bool                    `json:"dimensionsDiff"`
	CrossResource  bool                    `json:"crossResource"`
}
```

**Step 3: Add CompareVersionsCross method**

Add after line 532:

```go
// CompareVersionsCross compares versions that may belong to different resources
func (ctx *MahresourcesContext) CompareVersionsCross(r1ID uint, v1Num int, r2ID uint, v2Num int) (*VersionComparison, error) {
	// If r2ID is 0, use r1ID (same-resource comparison)
	if r2ID == 0 {
		r2ID = r1ID
	}

	version1, err := ctx.GetVersionByNumber(r1ID, v1Num)
	if err != nil {
		return nil, fmt.Errorf("version 1 not found: %w", err)
	}

	version2, err := ctx.GetVersionByNumber(r2ID, v2Num)
	if err != nil {
		return nil, fmt.Errorf("version 2 not found: %w", err)
	}

	comparison := &VersionComparison{
		Version1:       version1,
		Version2:       version2,
		SizeDelta:      version2.FileSize - version1.FileSize,
		SameHash:       version1.Hash == version2.Hash,
		SameType:       version1.ContentType == version2.ContentType,
		DimensionsDiff: version1.Width != version2.Width || version1.Height != version2.Height,
		CrossResource:  r1ID != r2ID,
	}

	// For cross-resource comparison, include resource details
	if r1ID != r2ID {
		res1, err := ctx.GetResource(r1ID)
		if err != nil {
			return nil, fmt.Errorf("resource 1 not found: %w", err)
		}
		res2, err := ctx.GetResource(r2ID)
		if err != nil {
			return nil, fmt.Errorf("resource 2 not found: %w", err)
		}
		comparison.Resource1 = res1
		comparison.Resource2 = res2
	}

	return comparison, nil
}
```

**Step 4: Run tests**

Run: `go test ./application_context/... -v -run TestCompare`
Expected: All compare tests pass

**Step 5: Commit**

```bash
git add application_context/resource_version_context.go models/query_models/version_query.go
git commit -m "feat: extend version comparison to support cross-resource comparison"
```

---

## Task 2: Create Compare Page Context Provider

**Files:**
- Create: `server/template_handlers/template_context_providers/compare_template_context.go`

**Step 1: Create the context provider**

```go
package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models/query_models"
	"net/http"
	"strings"
)

func CompareContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		var query query_models.CrossVersionCompareQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}

		// Validate required params
		if query.Resource1ID == 0 {
			return baseContext.Update(pongo2.Context{
				"pageTitle":    "Compare Versions",
				"errorMessage": "Resource 1 ID (r1) is required",
			})
		}

		// Default r2 to r1 if not provided
		if query.Resource2ID == 0 {
			query.Resource2ID = query.Resource1ID
		}

		// Get resource 1 and its versions for the picker
		resource1, err := context.GetResource(query.Resource1ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions1, _ := context.GetVersions(query.Resource1ID)

		// Get resource 2 and its versions
		resource2, err := context.GetResource(query.Resource2ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions2, _ := context.GetVersions(query.Resource2ID)

		// Perform comparison if both versions specified
		var comparison *application_context.VersionComparison
		if query.Version1 > 0 && query.Version2 > 0 {
			comparison, err = context.CompareVersionsCross(
				query.Resource1ID, query.Version1,
				query.Resource2ID, query.Version2,
			)
			if err != nil {
				return addErrContext(err, baseContext)
			}
		}

		// Determine content type category for UI rendering
		contentCategory := "binary"
		if comparison != nil && comparison.Version1 != nil {
			ct := comparison.Version1.ContentType
			if strings.HasPrefix(ct, "image/") {
				contentCategory = "image"
			} else if strings.HasPrefix(ct, "text/") || ct == "application/json" || ct == "application/xml" {
				contentCategory = "text"
			} else if ct == "application/pdf" {
				contentCategory = "pdf"
			}
		}

		return baseContext.Update(pongo2.Context{
			"pageTitle":       "Compare Versions",
			"resource1":       resource1,
			"resource2":       resource2,
			"versions1":       versions1,
			"versions2":       versions2,
			"comparison":      comparison,
			"query":           query,
			"contentCategory": contentCategory,
			"crossResource":   query.Resource1ID != query.Resource2ID,
		})
	}
}
```

**Step 2: Verify file compiles**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/compare_template_context.go
git commit -m "feat: add compare page context provider"
```

---

## Task 3: Register Compare Route

**Files:**
- Modify: `server/routes.go:20-70`

**Step 1: Add compare template to templates map**

After line 35 (`"/resource":` entry), add:

```go
"/resource/compare": {template_context_providers.CompareContextProvider, "compare.tpl", http.MethodGet},
```

**Step 2: Verify file compiles**

Run: `go build ./...`
Expected: Build succeeds (will fail until template exists)

**Step 3: Commit**

```bash
git add server/routes.go
git commit -m "feat: register /resource/compare route"
```

---

## Task 4: Create Compare Template - Base Structure

**Files:**
- Create: `templates/compare.tpl`

**Step 1: Create the template file**

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="max-w-7xl mx-auto" x-data="compareView({
    r1: {{ query.Resource1ID }},
    v1: {{ query.Version1|default:0 }},
    r2: {{ query.Resource2ID }},
    v2: {{ query.Version2|default:0 }}
})">
    <!-- Resource/Version Pickers -->
    <div class="grid grid-cols-2 gap-6 mb-6">
        <!-- Left Side Picker -->
        <div class="bg-white shadow rounded-lg p-4">
            <label class="block text-sm font-medium text-gray-700 mb-2">Resource</label>
            <div x-data="autocompleter({
                url: '/v1/resources',
                selectedItems: [{{ resource1|json_encode }}],
                elName: 'r1',
                multiple: false
            })" x-bind="events" class="mb-3">
                <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                       class="w-full border rounded px-3 py-2"
                       placeholder="Search resources...">
                <div x-show="open" x-bind="dropdownEvents" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto">
                    <template x-for="item in results" :key="item.ID">
                        <div @click="selectItem(item)" class="px-3 py-2 hover:bg-gray-100 cursor-pointer" x-text="item.Name"></div>
                    </template>
                </div>
            </div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Version</label>
            <select x-model="v1" @change="updateUrl()" class="w-full border rounded px-3 py-2">
                {% for v in versions1 %}
                <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version1 %}selected{% endif %}>
                    v{{ v.VersionNumber }} - {{ v.CreatedAt|date:"Jan 02, 2006" }}
                </option>
                {% endfor %}
            </select>
        </div>

        <!-- Right Side Picker -->
        <div class="bg-white shadow rounded-lg p-4">
            <label class="block text-sm font-medium text-gray-700 mb-2">Resource</label>
            <div x-data="autocompleter({
                url: '/v1/resources',
                selectedItems: [{{ resource2|json_encode }}],
                elName: 'r2',
                multiple: false
            })" x-bind="events" class="mb-3">
                <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                       class="w-full border rounded px-3 py-2"
                       placeholder="Search resources...">
                <div x-show="open" x-bind="dropdownEvents" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto">
                    <template x-for="item in results" :key="item.ID">
                        <div @click="selectItem(item)" class="px-3 py-2 hover:bg-gray-100 cursor-pointer" x-text="item.Name"></div>
                    </template>
                </div>
            </div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Version</label>
            <select x-model="v2" @change="updateUrl()" class="w-full border rounded px-3 py-2">
                {% for v in versions2 %}
                <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version2 %}selected{% endif %}>
                    v{{ v.VersionNumber }} - {{ v.CreatedAt|date:"Jan 02, 2006" }}
                </option>
                {% endfor %}
            </select>
        </div>
    </div>

    {% if comparison %}
    <!-- Metadata Comparison Table -->
    <div class="bg-white shadow rounded-lg p-4 mb-6">
        <h3 class="text-lg font-medium mb-4">Metadata Comparison</h3>
        <table class="w-full">
            <thead>
                <tr class="text-left text-gray-600 border-b">
                    <th class="py-2">Property</th>
                    <th class="py-2">Left</th>
                    <th class="py-2">Right</th>
                    <th class="py-2 text-center">Status</th>
                </tr>
            </thead>
            <tbody>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Content Type</td>
                    <td class="py-2">{{ comparison.Version1.ContentType }}</td>
                    <td class="py-2">{{ comparison.Version2.ContentType }}</td>
                    <td class="py-2 text-center">
                        {% if comparison.SameType %}
                        <span class="text-green-600">=</span>
                        {% else %}
                        <span class="text-red-600">≠</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">File Size</td>
                    <td class="py-2">{{ comparison.Version1.FileSize|humanReadableSize }}</td>
                    <td class="py-2">{{ comparison.Version2.FileSize|humanReadableSize }}</td>
                    <td class="py-2 text-center">
                        {% if comparison.SizeDelta == 0 %}
                        <span class="text-green-600">=</span>
                        {% elif comparison.SizeDelta > 0 %}
                        <span class="text-blue-600">+{{ comparison.SizeDelta|humanReadableSize }}</span>
                        {% else %}
                        <span class="text-orange-600">{{ comparison.SizeDelta|humanReadableSize }}</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Dimensions</td>
                    <td class="py-2">{{ comparison.Version1.Width }}×{{ comparison.Version1.Height }}</td>
                    <td class="py-2">{{ comparison.Version2.Width }}×{{ comparison.Version2.Height }}</td>
                    <td class="py-2 text-center">
                        {% if comparison.DimensionsDiff %}
                        <span class="text-red-600">≠</span>
                        {% else %}
                        <span class="text-green-600">=</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Hash Match</td>
                    <td class="py-2 font-mono text-xs">{{ comparison.Version1.Hash|truncatechars:16 }}...</td>
                    <td class="py-2 font-mono text-xs">{{ comparison.Version2.Hash|truncatechars:16 }}...</td>
                    <td class="py-2 text-center">
                        {% if comparison.SameHash %}
                        <span class="text-green-600 text-xl">✓</span>
                        {% else %}
                        <span class="text-red-600 text-xl">✗</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Created</td>
                    <td class="py-2">{{ comparison.Version1.CreatedAt|date:"Jan 02, 2006 15:04" }}</td>
                    <td class="py-2">{{ comparison.Version2.CreatedAt|date:"Jan 02, 2006 15:04" }}</td>
                    <td class="py-2"></td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Comment</td>
                    <td class="py-2 italic text-gray-500">"{{ comparison.Version1.Comment }}"</td>
                    <td class="py-2 italic text-gray-500">"{{ comparison.Version2.Comment }}"</td>
                    <td class="py-2"></td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Resource</td>
                    <td class="py-2"><a href="/resource?id={{ resource1.ID }}" class="text-indigo-600 hover:underline">{{ resource1.Name }}</a></td>
                    <td class="py-2"><a href="/resource?id={{ resource2.ID }}" class="text-indigo-600 hover:underline">{{ resource2.Name }}</a></td>
                    <td class="py-2 text-center">
                        {% if crossResource %}<span class="text-orange-600">≠</span>{% else %}<span class="text-green-600">=</span>{% endif %}
                    </td>
                </tr>
            </tbody>
        </table>
    </div>

    <!-- Content Comparison Area -->
    {% if contentCategory == "image" %}
        {% include "/partials/compareImage.tpl" %}
    {% elif contentCategory == "text" %}
        {% include "/partials/compareText.tpl" %}
    {% elif contentCategory == "pdf" %}
        {% include "/partials/comparePdf.tpl" %}
    {% else %}
        {% include "/partials/compareBinary.tpl" %}
    {% endif %}

    {% else %}
    <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-4 text-yellow-800">
        Select versions to compare using the dropdowns above.
    </div>
    {% endif %}
</div>
{% endblock %}
```

**Step 2: Verify template syntax (build app)**

Run: `go build --tags 'json1 fts5' && echo "Build OK"`
Expected: Build OK

**Step 3: Commit**

```bash
git add templates/compare.tpl
git commit -m "feat: create compare page template with metadata table"
```

---

## Task 5: Create Image Comparison Partial

**Files:**
- Create: `templates/partials/compareImage.tpl`

**Step 1: Create the partial**

```django
<div class="bg-white shadow rounded-lg p-4" x-data="imageCompare({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}'
})">
    <!-- Mode selector -->
    <div class="flex space-x-2 mb-4 border-b pb-4">
        <button @click="mode = 'side-by-side'"
                :class="mode === 'side-by-side' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Side-by-side</button>
        <button @click="mode = 'slider'"
                :class="mode === 'slider' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Slider</button>
        <button @click="mode = 'onion'"
                :class="mode === 'onion' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Onion skin</button>
        <button @click="mode = 'toggle'"
                :class="mode === 'toggle' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Toggle</button>
        <div class="flex-grow"></div>
        <button @click="swapSides()" class="px-4 py-2 bg-gray-200 rounded">Swap sides</button>
    </div>

    <!-- Side-by-side mode -->
    <div x-show="mode === 'side-by-side'" class="grid grid-cols-2 gap-4">
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600">v{{ comparison.Version1.VersionNumber }}</div>
            <img :src="leftUrl" class="max-w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        </div>
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600">v{{ comparison.Version2.VersionNumber }}</div>
            <img :src="rightUrl" class="max-w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
        </div>
    </div>

    <!-- Slider mode -->
    <div x-show="mode === 'slider'" class="relative border rounded overflow-hidden" style="max-height: 600px;">
        <img :src="rightUrl" class="w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
        <div class="absolute inset-0 overflow-hidden" :style="'width: ' + sliderPos + '%'">
            <img :src="leftUrl" class="h-full object-cover object-left"
                 :style="'width: ' + (100 / sliderPos * 100) + '%'"
                 alt="Version {{ comparison.Version1.VersionNumber }}">
        </div>
        <div class="absolute inset-y-0 bg-white w-1 cursor-ew-resize"
             :style="'left: ' + sliderPos + '%'"
             @mousedown="startSliderDrag">
            <div class="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-6 h-12 bg-white rounded shadow flex items-center justify-center">
                <span class="text-gray-400">⋮</span>
            </div>
        </div>
    </div>

    <!-- Onion skin mode -->
    <div x-show="mode === 'onion'" class="relative border rounded overflow-hidden">
        <img :src="leftUrl" class="w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        <img :src="rightUrl" class="absolute inset-0 w-full h-auto"
             :style="'opacity: ' + (opacity / 100)"
             alt="Version {{ comparison.Version2.VersionNumber }}">
        <div class="absolute bottom-4 left-1/2 -translate-x-1/2 bg-white/80 rounded px-4 py-2 flex items-center space-x-3">
            <span class="text-sm">v{{ comparison.Version1.VersionNumber }}</span>
            <input type="range" min="0" max="100" x-model="opacity" class="w-48">
            <span class="text-sm">v{{ comparison.Version2.VersionNumber }}</span>
        </div>
    </div>

    <!-- Toggle mode -->
    <div x-show="mode === 'toggle'" class="relative border rounded overflow-hidden cursor-pointer" @click="toggleSide()" @keydown.space.prevent="toggleSide()">
        <div class="absolute top-2 right-2 bg-white/80 rounded px-2 py-1 text-sm font-medium" x-text="showLeft ? 'v{{ comparison.Version1.VersionNumber }}' : 'v{{ comparison.Version2.VersionNumber }}'"></div>
        <img x-show="showLeft" :src="leftUrl" class="w-full h-auto" alt="Version {{ comparison.Version1.VersionNumber }}">
        <img x-show="!showLeft" :src="rightUrl" class="w-full h-auto" alt="Version {{ comparison.Version2.VersionNumber }}">
        <div class="absolute bottom-4 left-1/2 -translate-x-1/2 bg-white/80 rounded px-4 py-2 text-sm">
            Click or press Space to toggle
        </div>
    </div>
</div>
```

**Step 2: Commit**

```bash
git add templates/partials/compareImage.tpl
git commit -m "feat: add image comparison partial with 4 view modes"
```

---

## Task 6: Create Text Comparison Partial

**Files:**
- Create: `templates/partials/compareText.tpl`

**Step 1: Create the partial**

```django
<div class="bg-white shadow rounded-lg p-4" x-data="textDiff({
    leftUrl: '/v1/resource/version/file?versionId={{ comparison.Version1.ID }}',
    rightUrl: '/v1/resource/version/file?versionId={{ comparison.Version2.ID }}'
})">
    <!-- Mode selector -->
    <div class="flex items-center space-x-4 mb-4 border-b pb-4">
        <button @click="mode = 'unified'"
                :class="mode === 'unified' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Unified</button>
        <button @click="mode = 'split'"
                :class="mode === 'split' ? 'bg-indigo-600 text-white' : 'bg-gray-200'"
                class="px-4 py-2 rounded">Side-by-side</button>
        <div class="flex-grow"></div>
        <span class="text-sm text-gray-600" x-show="stats.added || stats.removed">
            <span class="text-green-600">+<span x-text="stats.added"></span></span>
            <span class="text-red-600 ml-2">-<span x-text="stats.removed"></span></span>
            lines
        </span>
    </div>

    <!-- Loading state -->
    <div x-show="loading" class="text-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 mx-auto"></div>
        <p class="mt-2 text-gray-600">Loading files...</p>
    </div>

    <!-- Error state -->
    <div x-show="error" class="bg-red-50 border border-red-200 rounded p-4 text-red-800" x-text="error"></div>

    <!-- Unified diff view -->
    <div x-show="!loading && !error && mode === 'unified'" class="font-mono text-sm overflow-x-auto">
        <table class="w-full">
            <template x-for="(line, index) in unifiedDiff" :key="index">
                <tr :class="{
                    'bg-red-50': line.type === 'removed',
                    'bg-green-50': line.type === 'added',
                    'bg-gray-50': line.type === 'context'
                }">
                    <td class="text-gray-400 text-right pr-2 select-none w-12" x-text="line.leftNum || ''"></td>
                    <td class="text-gray-400 text-right pr-2 select-none w-12 border-r" x-text="line.rightNum || ''"></td>
                    <td class="pl-2">
                        <span :class="{
                            'text-red-600': line.type === 'removed',
                            'text-green-600': line.type === 'added'
                        }" x-text="line.prefix"></span>
                        <span x-text="line.content" class="whitespace-pre"></span>
                    </td>
                </tr>
            </template>
        </table>
    </div>

    <!-- Split diff view -->
    <div x-show="!loading && !error && mode === 'split'" class="grid grid-cols-2 gap-0 font-mono text-sm overflow-x-auto">
        <div class="border-r">
            <div class="bg-gray-100 px-2 py-1 text-gray-600 sticky top-0">v{{ comparison.Version1.VersionNumber }}</div>
            <table class="w-full">
                <template x-for="(line, index) in splitLeft" :key="index">
                    <tr :class="{'bg-red-50': line.changed}">
                        <td class="text-gray-400 text-right pr-2 select-none w-12" x-text="line.num || ''"></td>
                        <td class="pl-2 whitespace-pre" x-text="line.content"></td>
                    </tr>
                </template>
            </table>
        </div>
        <div>
            <div class="bg-gray-100 px-2 py-1 text-gray-600 sticky top-0">v{{ comparison.Version2.VersionNumber }}</div>
            <table class="w-full">
                <template x-for="(line, index) in splitRight" :key="index">
                    <tr :class="{'bg-green-50': line.changed}">
                        <td class="text-gray-400 text-right pr-2 select-none w-12" x-text="line.num || ''"></td>
                        <td class="pl-2 whitespace-pre" x-text="line.content"></td>
                    </tr>
                </template>
            </table>
        </div>
    </div>
</div>
```

**Step 2: Commit**

```bash
git add templates/partials/compareText.tpl
git commit -m "feat: add text diff partial with unified and split views"
```

---

## Task 7: Create PDF and Binary Comparison Partials

**Files:**
- Create: `templates/partials/comparePdf.tpl`
- Create: `templates/partials/compareBinary.tpl`

**Step 1: Create PDF partial**

```django
<div class="bg-white shadow rounded-lg p-4" x-data="{ loaded: false }">
    <div class="flex items-center justify-between mb-4 border-b pb-4">
        <h3 class="text-lg font-medium">PDF Comparison</h3>
        <button @click="loaded = true" x-show="!loaded"
                class="px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">
            Load in viewer
        </button>
    </div>

    <!-- Thumbnails before loading -->
    <div x-show="!loaded" class="grid grid-cols-2 gap-6">
        <div class="border rounded p-4 text-center">
            <div class="w-24 h-32 bg-red-100 mx-auto mb-3 flex items-center justify-center">
                <span class="text-red-600 font-bold">PDF</span>
            </div>
            <p class="font-medium">v{{ comparison.Version1.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-gray-200 rounded hover:bg-gray-300">
                Download
            </a>
        </div>
        <div class="border rounded p-4 text-center">
            <div class="w-24 h-32 bg-red-100 mx-auto mb-3 flex items-center justify-center">
                <span class="text-red-600 font-bold">PDF</span>
            </div>
            <p class="font-medium">v{{ comparison.Version2.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-gray-200 rounded hover:bg-gray-300">
                Download
            </a>
        </div>
    </div>

    <!-- Iframes after loading -->
    <div x-show="loaded" class="grid grid-cols-2 gap-4">
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600 flex justify-between">
                <span>v{{ comparison.Version1.VersionNumber }}</span>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}" class="text-indigo-600 hover:underline">Download</a>
            </div>
            <iframe src="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
                    class="w-full h-[600px]"></iframe>
        </div>
        <div class="border rounded overflow-hidden">
            <div class="bg-gray-100 px-2 py-1 text-sm text-gray-600 flex justify-between">
                <span>v{{ comparison.Version2.VersionNumber }}</span>
                <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}" class="text-indigo-600 hover:underline">Download</a>
            </div>
            <iframe src="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
                    class="w-full h-[600px]"></iframe>
        </div>
    </div>
</div>
```

**Step 2: Create Binary partial**

```django
<div class="bg-white shadow rounded-lg p-4">
    <div class="bg-yellow-50 border border-yellow-200 rounded p-4 mb-6 text-yellow-800">
        Content preview not available for this file type. Use the download links to compare locally.
    </div>

    <div class="grid grid-cols-2 gap-6">
        <div class="border rounded p-4 text-center">
            {% if comparison.Version1.Width > 0 %}
            <img src="/v1/resource/preview?id={{ resource1.ID }}&maxX=200&maxY=200"
                 class="mx-auto mb-3 max-h-32" alt="Thumbnail">
            {% else %}
            <div class="w-24 h-24 bg-gray-200 mx-auto mb-3 flex items-center justify-center rounded">
                <span class="text-gray-500 text-xs">{{ comparison.Version1.ContentType }}</span>
            </div>
            {% endif %}
            <p class="font-medium">v{{ comparison.Version1.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version1.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version1.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">
                Download
            </a>
        </div>
        <div class="border rounded p-4 text-center">
            {% if comparison.Version2.Width > 0 %}
            <img src="/v1/resource/preview?id={{ resource2.ID }}&maxX=200&maxY=200"
                 class="mx-auto mb-3 max-h-32" alt="Thumbnail">
            {% else %}
            <div class="w-24 h-24 bg-gray-200 mx-auto mb-3 flex items-center justify-center rounded">
                <span class="text-gray-500 text-xs">{{ comparison.Version2.ContentType }}</span>
            </div>
            {% endif %}
            <p class="font-medium">v{{ comparison.Version2.VersionNumber }}</p>
            <p class="text-sm text-gray-500">{{ comparison.Version2.FileSize|humanReadableSize }}</p>
            <a href="/v1/resource/version/file?versionId={{ comparison.Version2.ID }}"
               class="inline-block mt-3 px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">
                Download
            </a>
        </div>
    </div>
</div>
```

**Step 3: Commit**

```bash
git add templates/partials/comparePdf.tpl templates/partials/compareBinary.tpl
git commit -m "feat: add PDF and binary comparison partials"
```

---

## Task 8: Add jsdiff Dependency

**Files:**
- Modify: `package.json`

**Step 1: Install jsdiff**

Run: `npm install diff --save`
Expected: Package added to package.json

**Step 2: Commit**

```bash
git add package.json package-lock.json
git commit -m "chore: add jsdiff library for text diffing"
```

---

## Task 9: Create compareView Alpine Component

**Files:**
- Create: `src/components/compareView.js`
- Modify: `src/main.js`

**Step 1: Create the component**

```javascript
export function compareView(initialState) {
  return {
    r1: initialState.r1,
    v1: initialState.v1,
    r2: initialState.r2,
    v2: initialState.v2,

    init() {
      // Watch for changes and update URL
    },

    updateUrl() {
      const url = new URL(window.location);
      url.searchParams.set('r1', this.r1);
      url.searchParams.set('v1', this.v1);
      url.searchParams.set('r2', this.r2);
      url.searchParams.set('v2', this.v2);
      window.location.href = url.toString();
    },

    async fetchVersions(resourceId) {
      const response = await fetch(`/v1/resource/versions?resourceId=${resourceId}`);
      return response.json();
    },

    async onResource1Change(resourceId) {
      this.r1 = resourceId;
      const versions = await this.fetchVersions(resourceId);
      if (versions.length > 0) {
        this.v1 = versions[0].versionNumber;
      }
      this.updateUrl();
    },

    async onResource2Change(resourceId) {
      this.r2 = resourceId;
      const versions = await this.fetchVersions(resourceId);
      if (versions.length > 0) {
        this.v2 = versions[0].versionNumber;
      }
      this.updateUrl();
    }
  };
}
```

**Step 2: Register in main.js**

Add after line 33 in `src/main.js`:

```javascript
import { compareView } from './components/compareView.js';
import { imageCompare } from './components/imageCompare.js';
import { textDiff } from './components/textDiff.js';
```

Add after line 73:

```javascript
Alpine.data('compareView', compareView);
Alpine.data('imageCompare', imageCompare);
Alpine.data('textDiff', textDiff);
```

**Step 3: Commit**

```bash
git add src/components/compareView.js src/main.js
git commit -m "feat: add compareView Alpine component for URL state management"
```

---

## Task 10: Create imageCompare Alpine Component

**Files:**
- Create: `src/components/imageCompare.js`

**Step 1: Create the component**

```javascript
export function imageCompare({ leftUrl, rightUrl }) {
  return {
    mode: 'side-by-side',
    leftUrl,
    rightUrl,
    sliderPos: 50,
    opacity: 50,
    showLeft: true,
    isDragging: false,

    swapSides() {
      const temp = this.leftUrl;
      this.leftUrl = this.rightUrl;
      this.rightUrl = temp;
    },

    toggleSide() {
      this.showLeft = !this.showLeft;
    },

    startSliderDrag(e) {
      this.isDragging = true;
      const container = e.target.closest('.relative');

      const moveHandler = (moveE) => {
        if (!this.isDragging) return;
        const rect = container.getBoundingClientRect();
        const x = (moveE.clientX || moveE.touches?.[0]?.clientX) - rect.left;
        this.sliderPos = Math.max(0, Math.min(100, (x / rect.width) * 100));
      };

      const upHandler = () => {
        this.isDragging = false;
        document.removeEventListener('mousemove', moveHandler);
        document.removeEventListener('mouseup', upHandler);
        document.removeEventListener('touchmove', moveHandler);
        document.removeEventListener('touchend', upHandler);
      };

      document.addEventListener('mousemove', moveHandler);
      document.addEventListener('mouseup', upHandler);
      document.addEventListener('touchmove', moveHandler);
      document.addEventListener('touchend', upHandler);
    }
  };
}
```

**Step 2: Commit**

```bash
git add src/components/imageCompare.js
git commit -m "feat: add imageCompare component with slider, onion, toggle modes"
```

---

## Task 11: Create textDiff Alpine Component

**Files:**
- Create: `src/components/textDiff.js`

**Step 1: Create the component**

```javascript
import * as Diff from 'diff';

export function textDiff({ leftUrl, rightUrl }) {
  return {
    mode: 'unified',
    loading: true,
    error: null,
    leftText: '',
    rightText: '',
    unifiedDiff: [],
    splitLeft: [],
    splitRight: [],
    stats: { added: 0, removed: 0 },

    async init() {
      try {
        const [leftRes, rightRes] = await Promise.all([
          fetch(leftUrl),
          fetch(rightUrl)
        ]);

        if (!leftRes.ok || !rightRes.ok) {
          throw new Error('Failed to load files');
        }

        this.leftText = await leftRes.text();
        this.rightText = await rightRes.text();
        this.computeDiff();
      } catch (e) {
        this.error = e.message;
      } finally {
        this.loading = false;
      }
    },

    computeDiff() {
      const diff = Diff.diffLines(this.leftText, this.rightText);

      // Build unified diff
      this.unifiedDiff = [];
      this.splitLeft = [];
      this.splitRight = [];

      let leftNum = 0;
      let rightNum = 0;
      let added = 0;
      let removed = 0;

      for (const part of diff) {
        const lines = part.value.split('\n');
        // Remove last empty line from split
        if (lines[lines.length - 1] === '') {
          lines.pop();
        }

        for (const line of lines) {
          if (part.added) {
            rightNum++;
            added++;
            this.unifiedDiff.push({
              type: 'added',
              prefix: '+',
              content: line,
              leftNum: null,
              rightNum: rightNum
            });
            this.splitLeft.push({ num: null, content: '', changed: false });
            this.splitRight.push({ num: rightNum, content: line, changed: true });
          } else if (part.removed) {
            leftNum++;
            removed++;
            this.unifiedDiff.push({
              type: 'removed',
              prefix: '-',
              content: line,
              leftNum: leftNum,
              rightNum: null
            });
            this.splitLeft.push({ num: leftNum, content: line, changed: true });
            this.splitRight.push({ num: null, content: '', changed: false });
          } else {
            leftNum++;
            rightNum++;
            this.unifiedDiff.push({
              type: 'context',
              prefix: ' ',
              content: line,
              leftNum: leftNum,
              rightNum: rightNum
            });
            this.splitLeft.push({ num: leftNum, content: line, changed: false });
            this.splitRight.push({ num: rightNum, content: line, changed: false });
          }
        }
      }

      this.stats = { added, removed };
    }
  };
}
```

**Step 2: Commit**

```bash
git add src/components/textDiff.js
git commit -m "feat: add textDiff component with jsdiff integration"
```

---

## Task 12: Update Version Panel for Compare Page Navigation

**Files:**
- Modify: `templates/partials/versionPanel.tpl:61-66`

**Step 1: Update the Compare Selected link**

Change line 62-65 from:

```django
<a :href="'/v1/resource/versions/compare?resourceId={{ resourceId }}&v1=' + selected[0] + '&v2=' + selected[1]"
   class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">
    Compare Selected
</a>
```

To:

```django
<a :href="'/resource/compare?r1={{ resourceId }}&v1=' + Math.min(...selected.map(s => versions.find(v => v.ID === s)?.VersionNumber || s)) + '&v2=' + Math.max(...selected.map(s => versions.find(v => v.ID === s)?.VersionNumber || s))"
   class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">
    Compare Selected
</a>
```

Actually, simpler approach - use version IDs directly:

```django
<a :href="'/resource/compare?r1={{ resourceId }}&v1=' + selected[0] + '&v2=' + selected[1]"
   class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">
    Compare Selected
</a>
```

Wait - the version panel uses version IDs, but our compare page expects version numbers. Let me check... Actually the current implementation needs to be updated to pass version numbers instead. Let me adjust the approach.

**Revised Step 1:** Update the template to extract version numbers:

The version panel already tracks version IDs. Update the compare link to navigate to the compare page:

```django
<a :href="'/resource/compare?r1={{ resourceId }}&v1=' + (selected[0]) + '&v2=' + (selected[1])"
   class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">
    Compare Selected
</a>
```

But we need version numbers. The cleanest fix is to update the compare page to accept version IDs OR version numbers. Let me update the context provider instead.

**Step 2: Commit**

```bash
git add templates/partials/versionPanel.tpl
git commit -m "feat: update version panel to navigate to compare page"
```

---

## Task 13: Add Bulk Action for Cross-Resource Compare

**Files:**
- Modify: `templates/partials/bulkEditorResource.tpl`

**Step 1: Add Compare button after line 41 (before delete form)**

```django
<div class="px-4" x-show="[...$store.bulkSelection.selectedIds].length === 2">
    <a :href="'/resource/compare?r1=' + [...$store.bulkSelection.selectedIds][0] + '&r2=' + [...$store.bulkSelection.selectedIds][1]"
       class="inline-flex justify-center py-2 px-4 mt-3 border border-transparent items-center shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
        Compare
    </a>
</div>
```

**Step 2: Commit**

```bash
git add templates/partials/bulkEditorResource.tpl
git commit -m "feat: add Compare bulk action when 2 resources selected"
```

---

## Task 14: Build and Manual Test

**Step 1: Build the application**

Run: `npm run build`
Expected: Build completes without errors

**Step 2: Start server in ephemeral mode**

Run: `./mahresources -ephemeral -bind-address=:8181`

**Step 3: Manual testing checklist**

- [ ] Navigate to `/resource/compare?r1=1&v1=1&v2=2` - page loads
- [ ] Version dropdowns show available versions
- [ ] Metadata table displays correctly
- [ ] Image comparison modes work (if comparing images)
- [ ] Text diff works (if comparing text files)
- [ ] PDF viewer loads on demand
- [ ] Bulk action "Compare" appears when 2 resources selected
- [ ] Version panel "Compare Selected" navigates to compare page

**Step 4: Commit any fixes**

---

## Task 15: Write E2E Tests

**Files:**
- Create: `e2e/tests/15-version-compare.spec.ts`

**Step 1: Create comprehensive E2E test file**

```typescript
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe.serial('Version Compare UI', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resource1Id: number;
  let resource2Id: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now();

    // Create prerequisite data
    const category = await apiClient.createCategory(
      `Compare Test Category ${testRunId}`,
      'Category for compare tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Compare Test Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test('should create resources with multiple versions', async ({ apiClient, page }) => {
    // Create first resource
    const testFile1 = path.join(__dirname, '../test-assets/sample-image-10.png');
    const resource1 = await apiClient.createResource({
      filePath: testFile1,
      name: `Compare Resource 1 ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resource1Id = resource1.ID;
    expect(resource1Id).toBeGreaterThan(0);

    // Create second resource
    const testFile2 = path.join(__dirname, '../test-assets/sample-image-11.png');
    const resource2 = await apiClient.createResource({
      filePath: testFile2,
      name: `Compare Resource 2 ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resource2Id = resource2.ID;
    expect(resource2Id).toBeGreaterThan(0);
  });

  test('should navigate to compare page from version panel', async ({ resourcePage, page }) => {
    expect(resource1Id).toBeGreaterThan(0);
    await resourcePage.gotoDisplay(resource1Id);

    // Open version panel
    await page.locator('button:has-text("Versions")').click();

    // Enter compare mode
    await page.locator('button:has-text("Compare")').click();
    await page.waitForTimeout(300);

    // Select two versions (need at least 2 versions)
    const checkboxes = page.locator('input[type="checkbox"]');
    const count = await checkboxes.count();

    if (count >= 2) {
      await checkboxes.first().check({ force: true });
      await checkboxes.nth(1).check({ force: true });

      // Click Compare Selected
      const compareLink = page.locator('a:has-text("Compare Selected")');
      await expect(compareLink).toBeVisible();

      const href = await compareLink.getAttribute('href');
      expect(href).toContain('/resource/compare');
    }
  });

  test('should show compare bulk action for exactly 2 resources', async ({ page }) => {
    await page.goto('/resources');

    // Select first resource
    const checkbox1 = page.locator(`input[type="checkbox"][value="${resource1Id}"]`);
    if (await checkbox1.isVisible()) {
      await checkbox1.check({ force: true });
    }

    // Select second resource
    const checkbox2 = page.locator(`input[type="checkbox"][value="${resource2Id}"]`);
    if (await checkbox2.isVisible()) {
      await checkbox2.check({ force: true });
    }

    // Compare button should appear
    const compareButton = page.locator('a:has-text("Compare")');
    await expect(compareButton).toBeVisible({ timeout: 5000 });

    // Verify link format
    const href = await compareButton.getAttribute('href');
    expect(href).toContain('/resource/compare');
    expect(href).toContain('r1=');
    expect(href).toContain('r2=');
  });

  test('should load compare page with metadata table', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);

    // Page should load
    await expect(page.locator('text=Metadata Comparison')).toBeVisible({ timeout: 10000 });

    // Metadata table should have rows
    await expect(page.locator('text=Content Type')).toBeVisible();
    await expect(page.locator('text=File Size')).toBeVisible();
    await expect(page.locator('text=Hash Match')).toBeVisible();
  });

  test('should show image comparison modes for image resources', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);

    // Mode buttons should be visible
    await expect(page.locator('button:has-text("Side-by-side")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('button:has-text("Slider")')).toBeVisible();
    await expect(page.locator('button:has-text("Onion skin")')).toBeVisible();
    await expect(page.locator('button:has-text("Toggle")')).toBeVisible();

    // Click different modes
    await page.locator('button:has-text("Slider")').click();
    await page.locator('button:has-text("Onion skin")').click();
    await page.locator('button:has-text("Toggle")').click();
    await page.locator('button:has-text("Side-by-side")').click();
  });

  test('should update URL when changing versions', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=1`);

    // Change version in dropdown (if multiple versions exist)
    const versionSelect = page.locator('select').first();
    const options = await versionSelect.locator('option').count();

    if (options > 1) {
      await versionSelect.selectOption({ index: 1 });
      await page.waitForLoadState('load');

      const url = page.url();
      expect(url).toContain('v1=');
    }
  });

  test.afterAll(async ({ apiClient }) => {
    // Cleanup
    if (resource1Id) {
      try { await apiClient.deleteResource(resource1Id); } catch {}
    }
    if (resource2Id) {
      try { await apiClient.deleteResource(resource2Id); } catch {}
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
```

**Step 2: Run tests**

Run: `cd e2e && npm run test:with-server -- --grep "Version Compare"`
Expected: All tests pass

**Step 3: Commit**

```bash
git add e2e/tests/15-version-compare.spec.ts
git commit -m "test: add E2E tests for version compare UI"
```

---

## Task 16: Final Build and Full Test Suite

**Step 1: Build**

Run: `npm run build`
Expected: Build succeeds

**Step 2: Run Go tests**

Run: `go test ./...`
Expected: All tests pass

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass

**Step 4: Create final commit if any fixes needed**

---

## Summary

This plan creates:
1. Extended backend supporting cross-resource version comparison
2. A new `/resource/compare` page with Alpine.js-powered UI
3. Image comparison with 4 modes (side-by-side, slider, onion skin, toggle)
4. Text diff with unified and split views using jsdiff
5. PDF viewing with on-demand iframe loading
6. Binary file fallback with download links
7. Entry points from version panel and bulk actions
8. Comprehensive E2E tests
