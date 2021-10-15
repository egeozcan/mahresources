<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css">
    <link rel="stylesheet" href="/public/tailwind.css">
    <script src="/public/index.js" defer></script>
    <script src="https://cdn.jsdelivr.net/gh/alpinejs/alpine@2.8.2/dist/alpine.js" integrity="sha256-9R44V6iCmVV7oDivSSvnPm4oYYirH6gC7ft09IS4j+o=" crossorigin="anonymous"></script>
    <script src="https://cdn.jsdelivr.net/npm/@ryangjchandler/spruce@2.6.3/dist/spruce.umd.js" integrity="sha256-nhO9lE7wv1L9WhmWs8WX3p8nwRGcVmimY9N6r2vzc60=" crossorigin="anonymous"></script>
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