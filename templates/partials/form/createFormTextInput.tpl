<div class="space-y-8 sm:space-y-5">
    <div>
        <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
            <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-gray-200">
                <label for="{{ name }}" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
                    {{ title }}
                </label>
                <div class="mt-1 sm:mt-0 sm:col-span-2">
                    <div class="max-w-lg flex rounded-md shadow-sm">
                        <input value="{{ value }}" {% if type %}type="{{ type }}"{% else %}type="text"{% endif %} {% if required %}required{% endif %} name="{{ name }}" id="{{ name }}" autocomplete="{{ name }}"
                               class="flex-1 block w-full focus:ring-indigo-500 focus:border-indigo-500 min-w-0 rounded-md sm:text-sm border-gray-300">
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>