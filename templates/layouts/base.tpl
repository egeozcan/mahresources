<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css">
    <link rel="stylesheet" href="/public/tailwind.css">
    <link rel="stylesheet" href="/public/jsonTable.css">
    <script defer src="https://cdn.jsdelivr.net/npm/@alpinejs/morph@3.13.0/dist/cdn.min.js" integrity="sha256-cZNlSCwrYgDRHHGVoiGiuvJq8Q8IYcQTRuCL5ROqKZQ=" crossorigin="anonymous"></script>
    <script defer src="https://cdn.jsdelivr.net/npm/@alpinejs/collapse@3.13.0/dist/cdn.min.js" integrity="sha256-K9XZcZtTfN2DuA4XH9cl2pzdr5lD1RD8tKwBQNs5pHo=" crossorigin="anonymous"></script>
    <script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.13.0/dist/cdn.min.js" integrity="sha256-OacPpuWbZSdnghMTo3qHPBlyIrpjY5ftBk1MmjrFOe0=" crossorigin="anonymous"></script>
    <script src="https://cdn.jsdelivr.net/npm/baguettebox.js@1.11.1/dist/baguetteBox.min.js" integrity="sha256-ULQV01VS9LCI2ePpLsmka+W0mawFpEA0rtxnezUj4A4=" crossorigin="anonymous"></script>
    <script src="/public/index.js"></script>
    <script src="/public/component.dropdown.js"></script>
    <script src="/public/component.confirmAction.js"></script>
    <script src="/public/component.freeFields.js"></script>
    <script src="/public/component.bulkSelection.js"></script>
    <script src="/public/component.storeConfig.js"></script>
    <script type="module" src="/public/webcomponent.expandabletext.js"></script>
    <script src="/public/tableMaker.js"></script>
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
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/baguettebox.js@1.11.1/dist/baguetteBox.min.css" integrity="sha256-cLMYWYYutHkt+KpNqjg7NVkYSQ+E2VbrXsEvOqU7mL0=" crossorigin="anonymous">
    <meta name="msapplication-TileColor" content="#ffffff">
    <meta name="msapplication-TileImage" content="/public/favicon/ms-icon-144x144.png">
    <meta name="theme-color" content="#ffffff">
    {% block head %}{% endblock %}
</head>
<body class="site">
    <header class="header flex justify-between align-middle">
        {% include "/partials/menu.tpl" %}
        <div x-cloak x-data="{ active: false }" class="settings relative inline-flex align-middle">
            <button class="text-lg" @click="active = !active" @click.outside="setTimeout(() => active = false, 100)">⚙</button>
            <div x-show="active" class="absolute p-4 mt-6 top-0 right-0 bg-white" style="max-width: 50vw; min-width: 170px;">
                <label class="flex justify-between items-center content-center">
                    Show Descriptions
                    <input type="checkbox" name="showDescriptions" x-data x-init="$store.savedSetting.registerEl($root)" />
                </label>
                {% block settings %}{% endblock %}
            </div>
        </div>
        {% block header %}{% endblock %}
    </header>
    {% include "/partials/title.tpl" %}
    <article class="content pb-16">
        <section class="sidebar">
            {% if mainEntity %}
            <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-400">Updated: </span>{{ mainEntity.UpdatedAt|date:"2006-01-02 15:04" }}</small>
            <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-gray-400">Created: </span>{{ mainEntity.CreatedAt|date:"2006-01-02 15:04" }}</small>
            {% endif %}
            {% block sidebar %}{% endblock %}
        </section>
        <section class="main">
            {% block prebody %}{% endblock %}
            {% block body %}{% endblock %}
        </section>
    </article>
    <footer class="footer sticky bottom-0 bg-white">
        {% include "/partials/pagination.tpl" %}
        {% block footer %}{% endblock %}
    </footer>
</body>
</html>