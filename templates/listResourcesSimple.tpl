{% extends "/layouts/gallery.tpl" %}

{% block head %}{% custom_css resources %}{% endblock %}

{% block top %}
    {# The contact sheet hides partials/title.tpl outright (`.simple .title` in    #}
    {# public/index.css): that section carries the page's action link and, on      #}
    {# other pages, a delete button and breadcrumb links, and merely un-painting   #}
    {# it left them focusable but invisible. The <h1> it also carried is the one   #}
    {# thing this page still needs, so it is reissued here -- inside <main>, so    #}
    {# axe's `region` rule is satisfied as well as `page-has-heading-one`, and     #}
    {# with no id, because `page-title` still belongs to the hidden heading and a  #}
    {# second element answering to it would fail `duplicate-id-aria`.              #}
    <h1 class="sr-only">{{ pageTitle }}</h1>
    <div class="my-4">{% include "/partials/boxSelect.tpl" with options=displayOptions %}</div>
    {% include "/partials/customListHeader.tpl" %}
    {% include "/partials/mrqlBar.tpl" with entity="resource" %}
{% endblock %}
{% block gallery %}
    {% for entity in resources %}
        {% include "/partials/resource.tpl" %}
    {% empty %}
        {% include "/partials/listEmpty.tpl" with label="resources" createUrl="/resource/new" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
<script>
    document.body.classList.add("simple")
</script>
{% endblock %}