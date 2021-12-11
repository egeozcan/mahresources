<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css">
    <link rel="stylesheet" href="/public/tailwind.css">
    <link rel="stylesheet" href="/public/jsonTable.css">
    <script src="https://cdn.jsdelivr.net/npm/alpinejs@3.4.2/dist/cdn.min.js" integrity="sha256-vtZIstyQ+MiaMGIEM80mS+F02WGC6ErZjQ/caLHUiO8=" crossorigin="anonymous" defer></script>
    <script src="/public/index.js"></script>
    <script src="/public/component.dropdown.js"></script>
    <script src="/public/component.confirmAction.js"></script>
    <script src="/public/component.freeFields.js"></script>
    <script src="/public/component.accordion.js"></script>
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
    <meta name="msapplication-TileColor" content="#ffffff">
    <meta name="msapplication-TileImage" content="/public/favicon/ms-icon-144x144.png">
    <meta name="theme-color" content="#ffffff">
    {% block head %}{% endblock %}
</head>
<body class="site">
    <header class="header">
        {% include "/partials/menu.tpl" %}
        {% block header %}{% endblock %}
    </header>
    {% include "/partials/title.tpl" %}
    <article class="content">
        <section class="sidebar">
            {% block sidebar %}{% endblock %}
        </section>
        <section class="main">
            {% block body %}{% endblock %}
        </section>
    </article>
    <footer class="footer">
        {% include "/partials/pagination.tpl" %}
        {% block footer %}{% endblock %}
    </footer>
</body>
</html>