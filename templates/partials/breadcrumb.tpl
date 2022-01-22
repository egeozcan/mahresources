<nav class="flex flex-wrap" aria-label="Breadcrumb">
  <ol role="list" class="bg-white rounded-md shadow px-6 flex flex-wrap flex-shrink space-x-4">
    <li class="flex">
      <div class="flex items-center">
        <a href="{{ HomeUrl }}" class="text-gray-400 hover:text-gray-500">
          <!-- Heroicon name: solid/home -->
          {% include "/partials/svg/home.tpl" %}
          <span class="sr-only">{{ HomeName }}</span>
        </a>
      </div>
    </li>
    {% for entry in Entries %}
    <li class="flex flex-shrink">
      <div class="flex items-center">
        {% include "/partials/svg/arrow.tpl" %}
        <a
          class="ml-4 text-sm font-medium text-gray-500 hover:text-gray-700 overflow-ellipsis whitespace-nowrap overflow-hidden max-w-sm "
          href="{{ entry.Url }}"
        >{{ entry.Name }}</a>
      </div>
    </li>
    {% endfor %}
  </ol>
</nav>