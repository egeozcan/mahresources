<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css">
    <script src="/public/index.js" defer></script>
    <script src="https://cdn.jsdelivr.net/gh/alpinejs/alpine@v2.8.2/dist/alpine.min.js" defer></script>
    {% block head %}{% endblock %}
</head>
<body class="site">
    <header class="header">
        <nav class="menu">
            {% for menuEntry in menu %}
            <a href="{{ menuEntry.Url }}" class="menu-item">
                {{ menuEntry.Name }}
            </a>
            {% endfor %}
            {% if action != nil %}
            <a href="{{ action.Url }}" class="menu-item">
                {{ action.Name }}
            </a>
            {% endif %}
        </nav>
        {% block header %}{% endblock %}
    </header>
    <article class="content">{% block body %}{% endblock %}</article>
    <footer class="footer">
        {% block footer %}{% endblock %}
        <p>Mahresources. </p>
    </footer>
</body>
</html>