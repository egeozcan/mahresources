{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-6" data-testid="admin-settings">
  <header class="space-y-1">
    {# Finding 110: partials/title.tpl renders the page <h1> from pageTitle #}
    {# ("Settings"); this was a second, differently-worded <h1>. #}
    <h2 class="text-lg font-semibold font-mono text-stone-800">Runtime Settings</h2>
    <p class="text-sm text-stone-500">
      Changes take effect immediately without a restart. Boot defaults are shown as reference and can be restored with Reset.
      {% if docsLinksEnabled %}
      <a href="{{ docsURL("configuration/runtime-settings") }}" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Runtime settings docs</a>
      {% endif %}
    </p>
  </header>

  {% for group in settingsByGroup %}
  <section class="rounded-lg bg-white border border-stone-200 p-5" aria-labelledby="grp-{{ group.Group }}">
    {# Finding 112: this printed group.Group — the raw config key — under a #}
    {# `capitalize` that cannot reach an underscore, so a reader saw #}
    {# "Remote_downloads". The key stays as the section id for aria-labelledby. #}
    <h2 id="grp-{{ group.Group }}" class="text-base font-semibold font-mono text-stone-800 mb-4">{{ group.Label }}</h2>
    <div class="space-y-4">
      {% for s in group.Items %}
      <div class="border border-stone-200 rounded-md p-4"
           data-setting-key="{{ s.Key }}"
           data-testid="setting-row-{{ s.Key }}"
           x-data="settingRow({{ s|json }})">
        <label :for="'setting-' + key" class="block text-sm font-medium text-stone-800">
          {{ s.Label }}
          <template x-if="overridden">
            <span class="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800">Override</span>
          </template>
        </label>
        <p class="text-xs text-stone-500 mt-0.5">{{ s.Description }}</p>
        {# Finding 33: typing a value and pressing Enter did nothing at all — no    #}
        {# save, no message — because these controls were not inside a <form>, so   #}
        {# there was nothing for Enter to submit. The value simply sat there looking #}
        {# edited (measured: hash_ahash_threshold current=5 overridden=False after   #}
        {# filling 6 and pressing Enter).                                           #}
        {#                                                                         #}
        {# Note the Save button is type="submit" below, and has to be: a form with   #}
        {# no submit button does not submit on Enter once more than one field blocks #}
        {# implicit submission, and this row has two text inputs (value and reason). #}
        {# novalidate, and it is load-bearing. Finding 115 made these number inputs   #}
        {# with min/max, so wrapping them in a form put native constraint validation  #}
        {# in front of Save: an out-of-bounds value was blocked with a browser bubble  #}
        {# and save() never ran, which took away the app's own inline message (the one #}
        {# that names the bounds and is announced through the row's live region) and    #}
        {# broke admin-settings.spec.ts's "out-of-bounds value shows inline error".     #}
        {# The min/max attributes stay — they drive the spinner and are exposed to      #}
        {# assistive tech — but the validation that speaks stays in charge.             #}
        <form class="mt-2 flex flex-wrap gap-2 items-start" novalidate @submit.prevent="save()">
          {# Finding 115: every setting rendered as a text box, so "abc" in an #}
          {# int64 field went to the server and came back 400 in the console. #}
          <input :id="'setting-' + key"
                 :type="inputType"
                 :min="inputMin"
                 :max="inputMax"
                 :step="inputType === 'number' ? '1' : null"
                 x-model="value"
                 class="border border-stone-300 rounded px-2 py-1 text-sm flex-1 min-w-[12rem] font-mono focus:outline-none focus:ring-2 focus:ring-amber-500"
                 :aria-describedby="'hint-' + key" />
          <input type="text"
                 placeholder="Reason (optional)"
                 x-model="reason"
                 class="border border-stone-300 rounded px-2 py-1 text-sm w-48 focus:outline-none focus:ring-2 focus:ring-amber-500"
                 :aria-label="'Reason for ' + label" />
          <button type="submit"
                  class="inline-flex items-center px-3 py-1 text-sm font-medium text-white bg-amber-700 rounded hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600">
            Save
          </button>
          <template x-if="overridden">
            <button type="button"
                    @click="reset()"
                    class="inline-flex items-center px-3 py-1 text-sm font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-stone-400">
              Reset
            </button>
          </template>
        </form>
        <p :id="'hint-' + key" class="text-xs text-stone-500 mt-1 font-mono">
          Boot default: <span x-text="bootDefaultDisplay"></span>
          <template x-if="minDisplay">
            <span class="ml-4">Min: <span x-text="minDisplay"></span></span>
          </template>
          <template x-if="maxDisplay">
            <span class="ml-4">Max: <span x-text="maxDisplay"></span></span>
          </template>
        </p>
        <div class="text-xs mt-1 min-h-[1.25rem]" role="status" aria-live="polite">
          <span x-show="flash" x-text="flash" class="text-green-700"></span>
          <span x-show="error" x-text="error" class="text-red-700"></span>
        </div>
      </div>
      {% endfor %}
    </div>
  </section>
  {% endfor %}

  {% if bootOnly %}
  <details class="rounded-lg bg-stone-50 border border-stone-200 p-5">
    <summary class="cursor-pointer text-sm font-medium font-mono text-stone-700 select-none">Boot-only settings (require restart to change)</summary>
    <table class="mt-3 text-sm w-full" aria-label="Boot-only settings">
      <thead>
        <tr>
          <th class="text-left p-2 text-xs font-medium text-stone-500 uppercase tracking-wider">Setting</th>
          <th class="text-left p-2 text-xs font-medium text-stone-500 uppercase tracking-wider">Value</th>
        </tr>
      </thead>
      <tbody>
        {% for f in bootOnly %}
        <tr class="border-t border-stone-200">
          <td class="p-2 text-stone-700">{{ f.Label }}</td>
          <td class="p-2 font-mono text-stone-900">{{ f.Value }}</td>
        </tr>
        {% endfor %}
      </tbody>
    </table>
  </details>
  {% endif %}
</div>

<script>
(function () {
  // The Go twin of this function is ShortDuration in
  // server/template_handlers/template_context_providers/admin_settings_template_context.go,
  // which /admin/export uses so the two pages cannot disagree (finding 104).
  // Change one and change the other.
  function nanosToShort(n) {
    if (typeof n !== 'number' || n === 0) return '0s';
    const ms = Math.floor(n / 1e6);
    if (ms < 1000) return ms + 'ms';
    const s = Math.floor(ms / 1000);
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    if (m < 60) return m + 'm' + (s % 60 ? (s % 60) + 's' : '');
    const h = Math.floor(m / 60);
    return h + 'h' + (m % 60 ? (m % 60) + 'm' : '');
  }

  function formatValue(type, val) {
    if (val === null || val === undefined) return '';
    if (type === 'duration') return nanosToShort(Number(val));
    return String(val);
  }

  window.settingRow = function settingRow(initial) {
    return {
      key: initial.key,
      label: initial.label,
      type: initial.type,
      value: formatValue(initial.type, initial.current),
      reason: '',
      overridden: initial.overridden,
      flash: '',
      error: '',

      // A duration is typed as "30s"/"5m", not as a number, so only the integer
      // types become spinbuttons; everything else stays a text box.
      get inputType() {
        return ['int', 'int64', 'uint64'].includes(this.type) ? 'number' : 'text';
      },
      get inputMin() {
        if (this.inputType !== 'number') return null;
        if (initial.minNumeric != null) return String(initial.minNumeric);
        return this.type === 'uint64' ? '0' : null;
      },
      get inputMax() {
        if (this.inputType !== 'number' || initial.maxNumeric == null) return null;
        return String(initial.maxNumeric);
      },

      get bootDefaultDisplay() {
        return formatValue(this.type, initial.bootDefault);
      },
      get minDisplay() {
        if (initial.minNumeric == null) return null;
        return formatValue(this.type, initial.minNumeric);
      },
      get maxDisplay() {
        if (initial.maxNumeric == null) return null;
        return formatValue(this.type, initial.maxNumeric);
      },

      async save() {
        this.error = '';
        this.flash = '';
        let res;
        try {
          res = await fetch('/v1/admin/settings/' + encodeURIComponent(this.key), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ value: this.value, reason: this.reason }),
          });
        } catch (e) {
          this.error = 'Network error: ' + e.message;
          return;
        }
        let body = null;
        try { body = await res.json(); } catch (_) { body = { error: 'HTTP ' + res.status }; }
        if (!res.ok) {
          this.error = (body && (body.error || body.message)) || 'HTTP ' + res.status;
          return;
        }
        this.overridden = body.overridden;
        this.value = formatValue(this.type, body.current);
        this.flash = 'Saved — took effect at ' + new Date().toLocaleTimeString();
      },

      async reset() {
        this.error = '';
        this.flash = '';
        let res;
        try {
          res = await fetch('/v1/admin/settings/' + encodeURIComponent(this.key), {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ reason: this.reason }),
          });
        } catch (e) {
          this.error = 'Network error: ' + e.message;
          return;
        }
        let body = null;
        try { body = await res.json(); } catch (_) { body = { error: 'HTTP ' + res.status }; }
        if (!res.ok) {
          this.error = (body && (body.error || body.message)) || 'HTTP ' + res.status;
          return;
        }
        this.overridden = false;
        this.value = formatValue(this.type, body.current);
        this.flash = 'Reset to boot default';
      },
    };
  };
}());
</script>
{% endblock %}
