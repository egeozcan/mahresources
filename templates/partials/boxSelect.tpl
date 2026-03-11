<div class="view-switcher mb-2" role="group" aria-label="Display options">
    {% for option in options %}
        <a href="{{ option.Link }}"
                {% if option.Active %}aria-current="true"{% endif %}
                class="view-switcher-option"
        >{{ option.Title }}</a>
    {% endfor %}
</div>
