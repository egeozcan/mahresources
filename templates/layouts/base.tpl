<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css">
    <link rel="stylesheet" href="/public/tailwind.css">
    <script src="/public/index.js" defer></script>
    <script src="https://cdn.jsdelivr.net/gh/alpinejs/alpine@v2.8.2/dist/alpine.min.js" defer></script>
    {% block head %}{% endblock %}
</head>
<body class="site">
    <header class="header">
        {% include "../partials/menu.tpl" %}
        {% block header %}{% endblock %}
    </header>
    {% include "../partials/title.tpl" %}
    <article class="content">
        <section class="sidebar">
            {% if tags %}
            <div class="tags mb-2" style="margin-left: -0.5rem">
                {% for tag in tags.Tags %}
                {% include "../partials/tag.tpl" %}
                {% endfor %}
            </div>
            {% endif %}
            {% block sidebar %}{% endblock %}
        </section>
        <section class="main">
            {% block body %}{% endblock %}
        </section>
    </article>
    <footer class="footer">
        {% include "../partials/pagination.tpl" %}
        {% block footer %}{% endblock %}
    </footer>
</body>
</html>