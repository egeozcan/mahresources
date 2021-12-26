<nav style="max-width: calc(100vw - 43px);" class="overflow-auto flex items-start content-start">
    {% for menuEntry in menu %}
    <a href="{{ menuEntry.Url }}" class="menu-item whitespace-nowrap h-8 inline-grid place-content-center place-items-center {% if menuEntry.Url == path %}font-bold{% endif %}">
        {{ menuEntry.Name }}
    </a>
    {% endfor %}
</nav>