<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ pageTitle }}</title>
    <link rel="stylesheet" href="/public/tailwind.css">
    <link rel="stylesheet" href="/public/index.css">
    <link rel="apple-touch-icon" sizes="180x180" href="/public/favicon/apple-icon-180x180.png">
    <link rel="icon" type="image/png" sizes="32x32" href="/public/favicon/favicon-32x32.png">
    <link rel="icon" type="image/png" sizes="16x16" href="/public/favicon/favicon-16x16.png">
    <meta name="theme-color" content="#ffffff">
    {% block head %}{% endblock %}
</head>
<body class="bg-gray-50 min-h-screen">
    <div class="max-w-4xl mx-auto py-8 px-4">
        {% block content %}{% endblock %}
    </div>
    <script type="module" src="/public/dist/main.js"></script>
</body>
</html>
