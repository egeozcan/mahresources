{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-6" method="post" action="/v1/reduction"
      x-data="reductionCreateForm" @submit.prevent="submit($el)" :aria-busy="busy">
    <p class="text-sm text-stone-600">
        Select groups or resources to compare. Matching settings are available after creation.
    </p>

    <fieldset class="space-y-6" :disabled="busy">
        <legend class="sr-only">New Resource Reduction</legend>
        <div class="max-w-lg">
            <label for="name" class="block text-xs font-mono font-medium text-stone-600">Name</label>
            <input type="text" name="name" id="name" autocomplete="off" aria-describedby="name-description"
                   class="mt-1 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
            <p id="name-description" class="mt-1 text-sm text-stone-500">Default: Resource Reduction.</p>
        </div>

        <div class="space-y-3">
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

        <div>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" name="excludeExternalResources" aria-describedby="reduction-external-hint"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Exclude external resources
            </label>
            <p id="reduction-external-hint" class="mt-1 text-xs text-stone-600">
                Only compare resources within the Extent.
            </p>
        </div>
    </fieldset>

    <p x-show="error" x-cloak x-text="error" role="alert" class="text-sm text-red-700"></p>

    <div class="flex justify-end gap-3 pt-5">
        <a href="/reductions" class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">Cancel</a>
        <button type="submit" :disabled="busy"
                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
            <span x-text="busy ? 'Creating…' : 'Create'">Create</span>
        </button>
    </div>
</form>
{% endblock %}
