{# flex-nowrap + min-w-0: the <ol> is the scroller (see .breadcrumb-list), and #}
{# a wrapping parent would let it grow instead of overflowing (finding 150).    #}
<nav class="flex flex-nowrap min-w-0" aria-label="Breadcrumb">
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
        {# Finding 150: the arrow separators are `w-6 h-full` full-height chevrons #}
        {# designed to read as one connected trail. When the crumbs wrap they do    #}
        {# not: measured at 390px the nav grew to 88px (two rows against 44px at    #}
        {# 1280px) and the second arrow sat at top:96 left:40 — stranded at the     #}
        {# left margin of its own line, connecting nothing. The class marks them so #}
        {# the narrow-viewport rule can swap them for an inline separator that is   #}
        {# part of the same flex line and therefore cannot detach.                  #}
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