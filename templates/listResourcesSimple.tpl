{% extends "/layouts/gallery.tpl" %}

{% block head %}{% custom_css resources %}{% endblock %}

{# The contact-sheet mode is a server-rendered body class, not one added from  #}
{# script. It used to be `document.body.classList.add("simple")` in an inline  #}
{# <script> here, which meant the mode -- and with it `.simple .title`, the    #}
{# rule that takes the original <h1> out of the accessibility tree -- did not  #}
{# exist at all with scripts blocked, and the page announced two <h1>s.        #}
{% block bodyClass %} simple{% endblock %}

{% block top %}
    {# The contact sheet hides partials/title.tpl outright, through the           #}
    {# `.simple .title` rule in public/index.css that the body class above turns   #}
    {# on. That section carries the page's action link and, on other pages, a      #}
    {# delete button and breadcrumb links, so merely un-painting it left them      #}
    {# focusable but invisible. The <h1> it also carried is the one thing this     #}
    {# page still needs, so it is reissued here -- inside <main>, which satisfies  #}
    {# axe's `region` rule as well as `page-has-heading-one`, and with no id,      #}
    {# because `page-title` still belongs to the hidden heading and a second       #}
    {# element answering to it would fail `duplicate-id-aria`.                     #}
    <h1 class="sr-only">{{ pageTitle }}</h1>
    <div class="my-4">{% include "/partials/boxSelect.tpl" with options=displayOptions %}</div>
    {% include "/partials/customListHeader.tpl" %}
    {% include "/partials/mrqlBar.tpl" with entity="resource" %}
{% endblock %}
{% block bottom %}
    {% include "/partials/customListFooter.tpl" %}
{% endblock %}

{% block gallery %}
    {% for entity in resources %}
        {% include "/partials/resource.tpl" %}
    {% empty %}
        {% include "/partials/listEmpty.tpl" with label="resources" createUrl="/resource/new" %}
    {% endfor %}
{% endblock %}
