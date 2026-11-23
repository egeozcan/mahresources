{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-6" method="post" action="/v1/reduction"
      x-data="reductionCreateForm" @submit.prevent="submit($el)" :aria-busy="busy">
    <p class="text-sm text-stone-600">
        Find copies of the same file and near-identical images, choose which copy to keep, and merge the rest into it.
        Nothing is removed until you review and apply.
    </p>

    <fieldset class="space-y-6" :disabled="busy">
        <legend class="sr-only">New Resource Reduction</legend>
        <div class="max-w-lg">
            <label for="name" class="block text-xs font-mono font-medium text-stone-600">Name</label>
            <input type="text" name="name" id="name" autocomplete="off" aria-describedby="name-description"
                   class="mt-1 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
            <p id="name-description" class="mt-1 text-sm text-stone-500">Default: Resource Reduction.</p>
        </div>

        <fieldset class="space-y-1">
            <legend class="block text-sm font-medium font-mono text-stone-700">Where to look</legend>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="radio" name="reductionScope" value="chosen" x-model="scope"
                       class="reduction-control border-stone-300 text-amber-700 focus:ring-amber-600">
                Choose groups and resources
            </label>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="radio" name="reductionScope" value="all" x-model="scope"
                       class="reduction-control border-stone-300 text-amber-700 focus:ring-amber-600">
                Everything you can access
            </label>
        </fieldset>

        <div class="space-y-3" x-show="scope === 'chosen'">
            <p id="reduction-selection-hint" class="text-sm text-stone-600">Choose at least one group or resource. Groups include resources from all subgroups.</p>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4" role="group" aria-label="Selection" aria-describedby="reduction-selection-hint">
                <div>
                    {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='group' categoryDecoration=true elName='groupIds' title='Groups' id='reduction-groups' %}
                </div>
                <div>
                    {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='resource' elName='resourceIds' title='Resources' id='reduction-resources' %}
                </div>
            </div>
        </div>

        <div x-show="scope === 'chosen'">
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" name="excludeExternalResources" aria-describedby="reduction-external-hint"
                       class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Only keep copies from this selection
            </label>
            <p id="reduction-external-hint" class="mt-1 text-xs text-stone-600">
                Otherwise a better copy elsewhere in your library may be kept instead.
            </p>
        </div>

        <details class="max-w-lg rounded border border-stone-200 p-3" data-testid="reduction-options">
            <summary class="cursor-pointer text-sm font-medium font-mono text-stone-700 focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600">Options</summary>
            <div class="mt-3 space-y-4">
                <fieldset class="space-y-1">
                    <legend class="block text-sm font-medium font-mono text-stone-700">Matching</legend>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="radio" name="matchingMode" value="both" x-model="matchingMode"
                               class="reduction-control border-stone-300 text-amber-700 focus:ring-amber-600">
                        Identical and Near-Identical Resources
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="radio" name="matchingMode" value="identical" x-model="matchingMode"
                               class="reduction-control border-stone-300 text-amber-700 focus:ring-amber-600">
                        Identical Resources only
                    </label>
                    <p class="text-xs text-stone-600">Near-Identical matching supports JPEG, PNG, GIF and WebP.</p>
                </fieldset>

                <div>
                    <label for="reduction-preset" class="block text-sm font-medium font-mono text-stone-700">Which copy to keep</label>
                    <select id="reduction-preset" x-model="preset" data-testid="reduction-preset"
                            class="reduction-control mt-1 block w-full max-w-xs rounded-md border-stone-300 shadow-sm text-sm focus:border-amber-600 focus:ring-amber-600">
                        <option value="best">Best quality</option>
                        <option value="smallest">Smallest file</option>
                        <option value="oldest">Oldest copy</option>
                        <option value="custom">Custom</option>
                    </select>
                    <p class="mt-1 text-xs text-stone-600" x-show="preset === 'best'">Highest resolution, then largest file, then added first.</p>
                    <p class="mt-1 text-xs text-stone-600" x-show="preset === 'smallest'" x-cloak>Smallest file, then highest resolution, then added first.</p>
                    <p class="mt-1 text-xs text-stone-600" x-show="preset === 'oldest'" x-cloak>Added first, then highest resolution, then largest file.</p>
                </div>

                <div x-show="preset === 'custom'" x-cloak data-testid="reduction-custom-rule">
                    <p class="text-xs text-stone-600">Choose criteria in priority order. Leave unused fields empty.</p>
                    <template x-for="(slot, i) in rule" :key="i">
                        <div class="mt-1">
                            <label class="block text-xs font-mono text-stone-600" :for="'create-rule-' + i" x-text="i === 0 ? 'First' : 'Then'"></label>
                            <select :id="'create-rule-' + i" x-model="rule[i]" data-testid="create-rule-slot"
                                    class="reduction-control mt-1 block w-full max-w-xs rounded-md border-stone-300 shadow-sm text-sm focus:border-amber-600 focus:ring-amber-600">
                                <option value="">None</option>
                                {% for criterion in winnerCriteria %}
                                <option value="{{ criterion.token }}">{{ criterion.label }}</option>
                                {% endfor %}
                            </select>
                        </div>
                    </template>
                </div>
            </div>
        </details>

        <div>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="computeNow" aria-describedby="reduction-compute-hint" data-testid="reduction-compute-now"
                       class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Find duplicates now
            </label>
            <p id="reduction-compute-hint" class="mt-1 text-xs text-stone-600">
                A Reduction tidies your library: one entry per image, with the others' tags, notes and groups merged in. It does not free disk space.
            </p>
        </div>
    </fieldset>

    <p x-show="error" x-cloak x-text="error" role="alert" class="text-sm text-red-700"></p>

    <div class="flex justify-end gap-3 pt-5">
        <a href="/reductions" class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">Cancel</a>
        <button type="submit" :disabled="busy"
                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
            <span x-text="busy ? 'Creating…' : 'Create'">Create</span>
        </button>
    </div>
</form>
{% endblock %}
