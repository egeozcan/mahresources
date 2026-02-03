<nav x-data="{ mobileOpen: false, adminOpen: false }" class="navbar flex items-center gap-1">
    <!-- Mobile hamburger -->
    <button @click="mobileOpen = !mobileOpen" class="navbar-toggle md:hidden" aria-label="Toggle menu">
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path x-show="!mobileOpen" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 6h16M4 12h16M4 18h16" />
            <path x-show="mobileOpen" x-cloak stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 18L18 6M6 6l12 12" />
        </svg>
    </button>

    <!-- Desktop navigation -->
    <div class="navbar-links hidden md:flex md:items-center md:gap-0.5">
        {% for menuEntry in menu %}
        <a href="{{ menuEntry.Url }}"
           class="navbar-link {% if menuEntry.Url == path %}navbar-link--active{% endif %}">
            {{ menuEntry.Name }}
        </a>
        {% endfor %}

        <!-- Admin dropdown -->
        <div class="navbar-dropdown" @click.outside="adminOpen = false">
            <button @click="adminOpen = !adminOpen"
                    class="navbar-link navbar-link--dropdown"
                    :class="{ 'navbar-link--active': adminOpen {% for adminEntry in adminMenu %}|| '{{ adminEntry.Url }}' == '{{ path }}'{% endfor %} }">
                <span>Admin</span>
                <svg class="navbar-dropdown-arrow" :class="{ 'rotate-180': adminOpen }" width="10" height="10" viewBox="0 0 10 10" fill="none">
                    <path d="M2 4L5 7L8 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
            </button>
            <div x-show="adminOpen"
                 x-cloak
                 x-transition:enter="transition ease-out duration-150"
                 x-transition:enter-start="opacity-0 -translate-y-1"
                 x-transition:enter-end="opacity-100 translate-y-0"
                 x-transition:leave="transition ease-in duration-100"
                 x-transition:leave-start="opacity-100 translate-y-0"
                 x-transition:leave-end="opacity-0 -translate-y-1"
                 class="navbar-dropdown-menu">
                {% for adminEntry in adminMenu %}
                <a href="{{ adminEntry.Url }}"
                   class="navbar-dropdown-item {% if adminEntry.Url == path %}navbar-dropdown-item--active{% endif %}"
                   @click="adminOpen = false">
                    {{ adminEntry.Name }}
                </a>
                {% endfor %}
            </div>
        </div>
    </div>

    <!-- Mobile navigation -->
    <div x-show="mobileOpen"
         x-cloak
         x-transition:enter="transition ease-out duration-200"
         x-transition:enter-start="opacity-0 -translate-y-2"
         x-transition:enter-end="opacity-100 translate-y-0"
         x-transition:leave="transition ease-in duration-150"
         x-transition:leave-start="opacity-100 translate-y-0"
         x-transition:leave-end="opacity-0 -translate-y-2"
         @click.outside="mobileOpen = false"
         class="navbar-mobile">

        <div class="navbar-mobile-section">
            {% for menuEntry in menu %}
            <a href="{{ menuEntry.Url }}"
               class="navbar-mobile-link {% if menuEntry.Url == path %}navbar-mobile-link--active{% endif %}"
               @click="mobileOpen = false">
                {{ menuEntry.Name }}
            </a>
            {% endfor %}
        </div>

        <div class="navbar-mobile-divider"></div>

        <div class="navbar-mobile-section">
            <span class="navbar-mobile-label">Admin</span>
            {% for adminEntry in adminMenu %}
            <a href="{{ adminEntry.Url }}"
               class="navbar-mobile-link {% if adminEntry.Url == path %}navbar-mobile-link--active{% endif %}"
               @click="mobileOpen = false">
                {{ adminEntry.Name }}
            </a>
            {% endfor %}
        </div>
    </div>
</nav>
