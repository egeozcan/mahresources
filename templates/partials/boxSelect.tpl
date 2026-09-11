{% if mrqlDisplay %}
<div class="view-switcher mb-2" role="group" aria-label="Display options">
    <button type="button" class="view-switcher-option" :aria-pressed="listLayout === 'cards'" :aria-current="listLayout === 'cards' ? 'true' : null" @click="listLayout = 'cards'">Cards</button>
    <button type="button" class="view-switcher-option" :aria-pressed="listLayout === 'list'" :aria-current="listLayout === 'list' ? 'true' : null" @click="listLayout = 'list'">Compact</button>
</div>
{% else %}
<div class="view-switcher mb-2" role="group" aria-label="Display options">
    {% for option in options %}
        <a href="{{ option.Link }}"
                {% if option.Active %}aria-current="true"{% endif %}
                class="view-switcher-option"
        >{{ option.Title }}</a>
    {% endfor %}
</div>

{% endif %}
