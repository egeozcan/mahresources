<nav x-data="{ open: false }" class="flex items-center">
    <!-- Mobile: hamburger -->
    <button @click="open = !open" class="md:hidden p-1 mr-1">
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
    </button>

    <!-- Desktop: inline links -->
    <div class="hidden md:flex md:items-center md:flex-wrap md:gap-x-1">
        {% for menuEntry in menu %}
        <a href="{{ menuEntry.Url }}" class="px-2 py-1 text-sm whitespace-nowrap {% if menuEntry.Url == path %}font-bold{% endif %}">{{ menuEntry.Name }}</a>
        {% endfor %}
    </div>

    <!-- Mobile: dropdown -->
    <div x-show="open" x-cloak @click.outside="open = false" class="md:hidden absolute left-0 top-full mt-1 bg-white shadow-lg ring-1 ring-black/5 z-50 rounded min-w-40">
        {% for menuEntry in menu %}
        <a href="{{ menuEntry.Url }}" class="block px-3 py-2 text-sm hover:bg-gray-50 {% if menuEntry.Url == path %}font-bold{% endif %}" @click="open = false">{{ menuEntry.Name }}</a>
        {% endfor %}
    </div>
</nav>
