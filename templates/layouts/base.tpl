<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{ title }}</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/public/index.css?v={{ assetVersion }}">
    <link rel="stylesheet" href="/public/tailwind.css?v={{ assetVersion }}">
    <link rel="stylesheet" href="/public/jsonTable.css?v={{ assetVersion }}">
    <script type="module" src="/public/dist/main.js?v={{ assetVersion }}"></script>
    <link rel="apple-touch-icon" sizes="57x57" href="/public/favicon/apple-icon-57x57.png">
    <link rel="apple-touch-icon" sizes="60x60" href="/public/favicon/apple-icon-60x60.png">
    <link rel="apple-touch-icon" sizes="72x72" href="/public/favicon/apple-icon-72x72.png">
    <link rel="apple-touch-icon" sizes="76x76" href="/public/favicon/apple-icon-76x76.png">
    <link rel="apple-touch-icon" sizes="114x114" href="/public/favicon/apple-icon-114x114.png">
    <link rel="apple-touch-icon" sizes="120x120" href="/public/favicon/apple-icon-120x120.png">
    <link rel="apple-touch-icon" sizes="144x144" href="/public/favicon/apple-icon-144x144.png">
    <link rel="apple-touch-icon" sizes="152x152" href="/public/favicon/apple-icon-152x152.png">
    <link rel="apple-touch-icon" sizes="180x180" href="/public/favicon/apple-icon-180x180.png">
    <link rel="icon" type="image/png" sizes="192x192"  href="/public/favicon/android-icon-192x192.png">
    <link rel="icon" type="image/png" sizes="32x32" href="/public/favicon/favicon-32x32.png">
    <link rel="icon" type="image/png" sizes="96x96" href="/public/favicon/favicon-96x96.png">
    <link rel="icon" type="image/png" sizes="16x16" href="/public/favicon/favicon-16x16.png">
    <meta name="msapplication-TileColor" content="#ffffff">
    <meta name="msapplication-TileImage" content="/public/favicon/ms-icon-144x144.png">
    <meta name="theme-color" content="#ffffff">
    {% block head %}{% endblock %}
    {% plugin_slot "head" %}
</head>
<body class="site bg-stone-50">
    <a href="#main-content" class="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:p-4 focus:bg-white focus:text-amber-700">Skip to main content</a>
    {% plugin_slot "page_top" %}
    <header class="header flex items-center justify-between gap-2 px-2">
        {% include "/partials/menu.tpl" %}
        <div class="flex items-center gap-1 flex-shrink-0">
            {% include "/partials/globalSearch.tpl" %}
            <div x-cloak x-data="{ active: false }" class="settings relative">
                <button class="p-1 text-lg" @click="active = !active" @click.outside="setTimeout(() => active = false, 100)" title="Settings" aria-label="Settings" :aria-expanded="active.toString()" aria-haspopup="true"><span aria-hidden="true">⚙</span></button>
                <div x-show="active" x-cloak class="absolute right-0 top-full mt-1 w-48 bg-white shadow-lg ring-1 ring-black/5 z-50 p-3 rounded">
                    <label class="flex justify-between items-center text-sm font-mono">
                        Show Descriptions
                        <input type="checkbox" name="showDescriptions" x-data x-init="$store.savedSetting.registerEl($root)" />
                    </label>
                    {% block settings %}{% endblock %}
                </div>
            </div>
        </div>
        {% block header %}{% endblock %}
    </header>
    {% include "/partials/title.tpl" %}
    <div class="content pb-16{% if dashboardPage %} content--no-sidebar{% endif %}" id="main-content">
        <aside class="sidebar">
            {% if mainEntity %}
            <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-stone-600 font-mono">Updated: </span>{{ mainEntity.UpdatedAt|date:"2006-01-02 15:04" }}</small>
            <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-stone-600 font-mono">Created: </span>{{ mainEntity.CreatedAt|date:"2006-01-02 15:04" }}</small>
            {% endif %}
            {% plugin_slot "sidebar_top" %}
            {% block sidebar %}{% endblock %}
            {% plugin_slot "sidebar_bottom" %}
        </aside>
        <main class="main">
            {% block prebody %}{% endblock %}
            {% block body %}{% endblock %}
        </main>
    </div>
    <footer class="footer sticky bottom-0 bg-white">
        {% include "/partials/pagination.tpl" %}
        {% block footer %}{% endblock %}
        {% plugin_slot "page_bottom" %}
    </footer>
    <div class="overlays">
        {% include "/partials/lightbox.tpl" %}
        {% include "/partials/pasteUpload.tpl" %}
        {% include "/partials/downloadCockpit.tpl" %}
        {% include "/partials/pluginActionModal.tpl" %}
        {% plugin_slot "scripts" %}
    </div>
</body>
</html>