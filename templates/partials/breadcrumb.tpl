<nav class="flex flex-wrap" aria-label="Breadcrumb">
  <ol role="list" class="breadcrumb-list bg-white rounded-md shadow px-6 flex flex-wrap flex-shrink space-x-4">
    <li class="flex">
      <div class="flex items-center">
        <a href="{{ HomeUrl }}" class="text-stone-600 hover:text-stone-700">
          <!-- Heroicon name: solid/home -->
          {% include "/partials/svg/home.tpl" %}
          <span class="sr-only">{{ HomeName }}</span>
        </a>
      </div>
    </li>
    {% for entry in Entries %}
    <li class="flex flex-shrink min-w-0">
      <div class="flex items-center min-w-0">
        {% include "/partials/svg/arrow.tpl" %}
        <a
          class="breadcrumb-link ml-4 text-sm font-mono font-medium text-stone-500 hover:text-stone-700 overflow-ellipsis whitespace-nowrap overflow-hidden"
          href="{{ entry.Url }}"
          {% if forloop.Last %}aria-current="page"{% endif %}
        >{{ entry.Name }}</a>
      </div>
    </li>
    {% endfor %}
  </ol>
</nav>