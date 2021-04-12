
<div
    x-data="{
        results: [],
        selectedIndex: -1,
        dropdownActive: false,
        selectedResults: {{ selectedItems|json }} || [],
        selectedIds: new Set(),
        url: '{{ url }}',
        requestAborter: null,
        addEl: (val, el) => el.appendChild(Object.assign(document.createElement('input'), {
            type: 'hidden',
            name: '{{ elName }}',
            value: val.ID
        })),
    }"
    x-init="
        selectedResults.forEach(val => {
            addEl(val, $refs.inputs);
            selectedIds.add(val.ID);
        });
        $watch('selectedResults', values => {
            $refs.inputs.innerHTML = '';
            selectedIds.clear();
            values.forEach(val => {
                addEl(val, $refs.inputs);
                selectedIds.add(val.ID);
            });
        });
    "
    class="relative"
>
    <label class="block text-sm font-medium text-gray-700 mt-3" for="{{ id }}">{{ title }}</label>
    <input
        id="{{ id }}"
        type="text"
        class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-2"
        @keydown.arrow-up.prevent="selectedIndex = selectedIndex - 1; if (selectedIndex < 0) selectedIndex = results.length - 1;"
        @keydown.arrow-down.prevent="selectedIndex = (selectedIndex + 1) % results.length"
        @keydown.enter.prevent="
            if (!results[selectedIndex]) return;
            selectedResults.push(results[selectedIndex]);
            $event.target.dispatchEvent(new Event('input'));
        "
        @blur="
            if (document.activeElement === $event.target) return;
            setTimeout(() => {
                dropdownActive = false
            }, 200)
        "
        @focus="dropdownActive = true; $event.target.dispatchEvent(new Event('input'))"
        @input="
            const target = $event.target;
            const value = target.value;

            results = results.filter(val => !selectedIds.has(val.ID));

            if (requestAborter) {
                requestAborter();
                requestAborter = null;
            }

            const { abort, ready } = abortableFetch(url + '?name=' + target.value, {})

            ready.then(x => x.json()).then(values => {
                if (value !== target.value) { return; }
                results = values.filter(val => !selectedIds.has(val.ID));

                if (results.length && document.activeElement === target) {
                    dropdownActive = true;
                    selectedIndex = 0;
                }
            });

            requestAborter = abort;
        "
    >
    <template x-for="(result, index) in selectedResults">
        <p class="
            inline-flex rounded-md items-center py-0.5 pl-2.5 pr-1 text-sm font-medium bg-indigo-100
            text-indigo-700 my-1
        ">
            <span class="break-all" x-text="result.Name"></span>
            <button
                    @click="selectedResults.splice(index, 1);"
                    type="button"
                    class="
                        flex-shrink-0 ml-0.5 h-4 w-4 rounded-md inline-flex items-center justify-center
                        text-indigo-400 hover:bg-indigo-200 hover:text-indigo-500 focus:outline-none
                        focus:bg-indigo-500 focus:text-white"
            >
                <span x-text="'Remove ' + result.Name" class="sr-only"></span>
                <svg class="h-2 w-2" stroke="currentColor" fill="none" viewBox="0 0 8 8">
                    <path stroke-linecap="round" stroke-width="1.5" d="M1 1l6 6m0-6L1 7" />
                </svg>
            </button>
        </p>
    </template>
    <div x-ref="inputs"></div>
    <template x-if="dropdownActive && results.length > 0">
        <div class="absolute mt-1 w-full border bg-white shadow-xl rounded z-50" style="top:3.8rem">
            <div class="p-3">
                <div x-ref="list">
                    <template x-for="(result, index) in results" :key="index">
                        <span
                            :active="false"
                            class="cursor-pointer p-2 flex block w-full rounded"
                            :class="{'bg-blue-500': index === selectedIndex}"
                            @click="selectedResults.push(result); dropdownActive = false;"
                            @mouseover="selectedIndex = index; console.log(selectedIndex);"
                        >
                            <span
                                x-text="result.Name"
                            ></span>
                        </span>
                    </template>
                </div>
            </div>
        </div>
    </template>
</div>
