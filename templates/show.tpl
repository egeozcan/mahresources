{% extends "layouts/base.tpl" %}

{% block body %}
<script>
    const img = document.createElement("img");
    (async function() {
        const res = await fetch("/v1/resource?id=1").then(x => x.json());
        img.src = 'data:' + res.ContentType + ';base64,' + res.Preview;
        document.body.append(img);
    })();
</script>
{% endblock %}