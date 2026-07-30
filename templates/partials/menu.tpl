{# Findings 116/121: the highlight was an exact `menuEntry.Url == path` match, so   #}
{# every detail page highlighted nothing, and no link ever carried aria-current —   #}
{# the current location was conveyed by colour alone, while pagination.tpl has done #}
{# it correctly all along. `activeNavUrl` is the *section* the URL belongs to (see   #}
{# activeNavURL in static_template_context.go); `activeNavIsExact` distinguishes     #}
{# "you are on this page" (aria-current="page") from "you are inside this section"   #}
{# (aria-current="true"), because claiming /resources is the current page while the  #}
{# reader is on /resource?id=63 would be a false statement, not a nicety.            #}
<nav aria-label="Main" x-data="mobileNav()"
     x-init="initMobileNav()"
     data-current-path="{{ path }}"
     data-active-nav="{{ activeNavUrl }}"
     class="navbar flex items-center gap-1">
    {# Finding 3: the mobile panel could not be closed. The toggle is the one #}
    {# affordance that has to survive the panel being painted over it, so it  #}
    {# sits above the panel's z-index; see .navbar-toggle in public/index.css. #}
    <!-- Mobile hamburger -->
    <button @click="toggleMobileNav($event)" class="navbar-toggle" aria-label="Toggle menu"
            :aria-expanded="mobileOpen.toString()" aria-controls="navbar-mobile-panel">
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path x-show="!mobileOpen" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 6h16M4 12h16M4 18h16" />
            <path x-show="mobileOpen" x-cloak stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 18L18 6M6 6l12 12" />
        </svg>
    </button>

    <!-- Desktop navigation -->
    <div class="navbar-links">
        {% for menuEntry in menu %}
        <a href="{{ menuEntry.Url }}"
           class="navbar-link {% if menuEntry.Url == activeNavUrl %}navbar-link--active{% endif %}"
           {% if menuEntry.Url == activeNavUrl %}aria-current="{% if activeNavIsExact %}page{% else %}true{% endif %}"{% endif %}>
            {{ menuEntry.Name }}
        </a>
        {% endfor %}

        <!-- Admin dropdown (editor and above; the server enforces per-item access) -->
        {% if not authEnabled or currentUser.CanEditorWrite %}
        <div class="navbar-dropdown" @click.outside="adminOpen = false" @keydown.escape="if (adminOpen) { adminOpen = false; $el.querySelector('button').focus(); }">
            <button @click="adminOpen = !adminOpen"
                    class="navbar-link navbar-link--dropdown"
                    :class="{ 'navbar-link--active': adminOpen {% for adminEntry in adminMenu %}|| '{{ adminEntry.Url }}' == activeNav{% endfor %} || '/admin/users' == activeNav }"
                    :aria-expanded="adminOpen.toString()"
                    aria-haspopup="true">
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
                   class="navbar-dropdown-item {% if adminEntry.Url == activeNavUrl %}navbar-dropdown-item--active{% endif %}"
                   {% if adminEntry.Url == activeNavUrl %}aria-current="{% if activeNavIsExact %}page{% else %}true{% endif %}"{% endif %}
                   @click="adminOpen = false">
                    {{ adminEntry.Name }}
                </a>
                {% endfor %}
                {% if not authEnabled or currentUser.IsAdmin %}
                <a href="/admin/users"
                   class="navbar-dropdown-item {% if '/admin/users' == activeNavUrl %}navbar-dropdown-item--active{% endif %}"
                   {% if '/admin/users' == activeNavUrl %}aria-current="{% if activeNavIsExact %}page{% else %}true{% endif %}"{% endif %}
                   @click="adminOpen = false">
                    Users
                </a>
                {% endif %}
            </div>
        </div>
        {% endif %}

        {% if hasPluginManager %}
        <div class="navbar-dropdown" @click.outside="pluginsOpen = false" @keydown.escape="if (pluginsOpen) { pluginsOpen = false; $el.querySelector('button').focus(); }">
            <button @click="pluginsOpen = !pluginsOpen"
                    class="navbar-link navbar-link--dropdown"
                    :class="{ 'navbar-link--active': pluginsOpen || '/plugins/manage' == currentPath {% for pi in pluginMenuItems %}|| '{{ pi.FullPath }}' == currentPath{% endfor %} }"
                    :aria-expanded="pluginsOpen.toString()"
                    aria-haspopup="true">
                <span>Plugins</span>
                <svg class="navbar-dropdown-arrow" :class="{ 'rotate-180': pluginsOpen }" width="10" height="10" viewBox="0 0 10 10" fill="none">
                    <path d="M2 4L5 7L8 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
            </button>
            <div x-show="pluginsOpen"
                 x-cloak
                 x-transition:enter="transition ease-out duration-150"
                 x-transition:enter-start="opacity-0 -translate-y-1"
                 x-transition:enter-end="opacity-100 translate-y-0"
                 x-transition:leave="transition ease-in duration-100"
                 x-transition:leave-start="opacity-100 translate-y-0"
                 x-transition:leave-end="opacity-0 -translate-y-1"
                 class="navbar-dropdown-menu">
                <a href="/plugins/manage"
                   class="navbar-dropdown-item {% if '/plugins/manage' == currentPath %}navbar-dropdown-item--active{% endif %}"
                   @click="pluginsOpen = false">
                    Manage Plugins
                </a>
                {% if pluginMenuItems %}
                <div class="navbar-dropdown-divider" role="separator"></div>
                {% endif %}
                {% for pi in pluginMenuItems %}
                <a href="{{ pi.FullPath }}"
                   class="navbar-dropdown-item {% if pi.FullPath == path %}navbar-dropdown-item--active{% endif %}"
                   @click="pluginsOpen = false">
                    {{ pi.Label }}
                </a>
                {% endfor %}
            </div>
        </div>
        {% endif %}
    </div>

    <!-- Mobile navigation -->
    {# Finding 3: Escape was inert, the panel held zero buttons, and it painted #}
    {# over the hamburger so the toggle click was intercepted.                  #}
    {# Deliberately NOT role="dialog" + aria-modal="true": this element is in    #}
    {# every page's DOM, and the lightbox is addressed app-wide by a strict      #}
    {# [role="dialog"][aria-modal="true"] locator whose two :not() exclusions    #}
    {# name the only other always-present modals. A third would be a            #}
    {# strict-mode violation in ~45 specs, not a soft failure. x-trap gives the  #}
    {# modal behaviour; the semantics stay a plain labelled region.              #}
    <div x-show="mobileOpen"
         id="navbar-mobile-panel"
         x-cloak
         x-transition:enter="transition ease-out duration-200"
         x-transition:enter-start="opacity-0 -translate-y-2"
         x-transition:enter-end="opacity-100 translate-y-0"
         x-transition:leave="transition ease-in duration-150"
         x-transition:leave-start="opacity-100 translate-y-0"
         x-transition:leave-end="opacity-0 -translate-y-2"
         @click.outside="closeMobileNav()"
         @keydown.escape.window="closeMobileNav()"
         x-trap.noscroll.noreturn="mobileOpen"
         role="group"
         aria-label="Site navigation"
         class="navbar-mobile">

        {# .noreturn, then an explicit deferred restore in mobileNav.js: x-trap #}
        {# activates on a setTimeout(15) and records whatever has focus then as #}
        {# its return node, which is the close button this panel focuses first. #}
        <div class="navbar-mobile-header">
            <button type="button" @click="closeMobileNav()" class="navbar-mobile-close" aria-label="Close menu">
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 18L18 6M6 6l12 12" />
                </svg>
            </button>
        </div>

        <div class="navbar-mobile-section">
            {% for menuEntry in menu %}
            <a href="{{ menuEntry.Url }}"
               class="navbar-mobile-link {% if menuEntry.Url == activeNavUrl %}navbar-mobile-link--active{% endif %}"
               {% if menuEntry.Url == activeNavUrl %}aria-current="{% if activeNavIsExact %}page{% else %}true{% endif %}"{% endif %}
               @click="mobileOpen = false">
                {{ menuEntry.Name }}
            </a>
            {% endfor %}
        </div>

        {% if not authEnabled or currentUser.CanEditorWrite %}
        <div class="navbar-mobile-divider"></div>

        <div class="navbar-mobile-section">
            <span class="navbar-mobile-label">Admin</span>
            {% for adminEntry in adminMenu %}
            <a href="{{ adminEntry.Url }}"
               class="navbar-mobile-link {% if adminEntry.Url == activeNavUrl %}navbar-mobile-link--active{% endif %}"
               {% if adminEntry.Url == activeNavUrl %}aria-current="{% if activeNavIsExact %}page{% else %}true{% endif %}"{% endif %}
               @click="mobileOpen = false">
                {{ adminEntry.Name }}
            </a>
            {% endfor %}
            {% if not authEnabled or currentUser.IsAdmin %}
            <a href="/admin/users"
               class="navbar-mobile-link {% if '/admin/users' == activeNavUrl %}navbar-mobile-link--active{% endif %}"
               {% if '/admin/users' == activeNavUrl %}aria-current="{% if activeNavIsExact %}page{% else %}true{% endif %}"{% endif %}
               @click="mobileOpen = false">
                Users
            </a>
            {% endif %}
        </div>
        {% endif %}

        {% if hasPluginManager %}
        <div class="navbar-mobile-divider"></div>

        <div class="navbar-mobile-section">
            <span class="navbar-mobile-label">Plugins</span>
            <a href="/plugins/manage"
               class="navbar-mobile-link {% if '/plugins/manage' == currentPath %}navbar-mobile-link--active{% endif %}"
               @click="mobileOpen = false">
                Manage Plugins
            </a>
            {% for pi in pluginMenuItems %}
            <a href="{{ pi.FullPath }}"
               class="navbar-mobile-link {% if pi.FullPath == path %}navbar-mobile-link--active{% endif %}"
               @click="mobileOpen = false">
                {{ pi.Label }}
            </a>
            {% endfor %}
        </div>
        {% endif %}
    </div>
</nav>
