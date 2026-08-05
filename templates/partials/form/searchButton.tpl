{# Finding 102, second instance. This wrapper was `sticky bottom-12 bg-stone-50   #}
{# pt-3 z-10 w-full`, which floated the submit 48px above the viewport bottom and #}
{# painted it over whichever sidebar filter field was there. Measured at          #}
{# 1280x720, a control was covered on 6 of the 37 pages in the a11y sweep —       #}
{# /notes 33px, /groups 19px, /resources 38px, /resources/details 38px,           #}
{# /groups/text 14px, /logs 7px — and `elementFromPoint` inside the overlap       #}
{# returned the button, not the field, so those clicks submitted the form instead #}
{# of focusing the input.                                                         #}
{#                                                                                #}
{# base.tpl and ws10_global_chrome_test.go's TestFooter_IsNotSticky already ruled #}
{# on exactly this: "a bar fixed to the viewport bottom covers page content at    #}
{# every scroll offset", measured on the /logs "After" date input. This partial   #}
{# reproduced it on that same input, so the declaration is dropped here for the   #}
{# same reason rather than made to work. A bottom-stuck element inside a          #}
{# page-scrolled column always renders above its own preceding siblings; the only #}
{# fix that keeps the pin is to make the sidebar its own scroll container, which  #}
{# is a layout change across 37 templates and the mobile disclosure.              #}
{#                                                                                #}
{# `bg-stone-50` went with it: it existed only to make the pinned bar opaque, and #}
{# below 1024px the disclosure is #fff, so it was painting a stray grey band.     #}
{# Guarded by e2e/tests/regressions/sidebar-apply-button-covers-filters.spec.ts   #}
{# (geometry) and TestSidebarSubmit_IsNotSticky (source).                         #}
{% if not small %}<div class="pt-3 w-full">{% endif %}
{# `disabledWhen` is an Alpine expression evaluated in the surrounding form's #}
{# scope. Used by the merge and Add Tags forms so the submit is unavailable #}
{# while the selection is empty (findings 16/92, 56/91). #}
<button type="submit"
    {% if disabledWhen %}:disabled="{{ disabledWhen }}"{% endif %}
    {% if describedBy %}aria-describedby="{{ describedBy }}"{% endif %}
    class="
    w-full inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-1.5 px-3
    {% endif %}
    border border-transparent
    text-xs font-mono font-semibold tracking-wide rounded text-white
    {% if danger %}
        bg-red-700 hover:bg-red-800 focus:ring-red-600
    {% else %}
        bg-amber-700 hover:bg-amber-800 focus:ring-amber-600
    {% endif %}
    focus:outline-none focus:ring-2 focus:ring-offset-1
    disabled:opacity-50 disabled:cursor-not-allowed
    transition-colors duration-100">
    {% if text %}{% autoescape off %}{{ text }}{% endautoescape %}{% else %}Apply Filters{% endif %}
</button>
{% if not small %}</div>{% endif %}
