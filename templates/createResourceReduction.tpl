{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-6" method="post" action="/v1/reduction"
      x-data="reductionCreateForm" @submit.prevent="submit($el)" :aria-busy="busy">
    <p class="text-sm text-stone-600">
        Select groups, resources, or both to start a Resource Reduction. You can adjust matching settings on the review page before computing.
    </p>

    <fieldset class="space-y-6" :disabled="busy">
        <legend class="sr-only">New Resource Reduction</legend>
        {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" description="Optional. Leave blank to use Resource Reduction." %}

        <div class="space-y-3">
            <p id="reduction-selection-hint" class="text-sm text-stone-600">Choose at least one group or resource. Groups include resources from their descendants when you compute.</p>
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
                Only compare resources within this Reduction's Extent. Matches outside it will not be considered.
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
