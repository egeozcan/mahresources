{# with note=note shareEnabled=shareEnabled shareBaseUrl=shareBaseUrl shareUrlConfigured=shareUrlConfigured #}
{% if shareEnabled %}
<div class="mt-4 pt-4 border-t border-stone-200">
    {% include "/partials/sideTitle.tpl" with title="Sharing" %}
    <div x-data="{
        shared: {% if note.ShareToken %}true{% else %}false{% endif %},
        shareToken: '{{ note.ShareToken|default:'' }}',
        shareBaseUrl: '{{ shareBaseUrl|default:'' }}',
        shareUrlConfigured: {% if shareUrlConfigured %}true{% else %}false{% endif %},
        loading: false,
        error: null,
        async share() {
            this.loading = true;
            this.error = null;
            try {
                const response = await fetch('/v1/note/share?noteId={{ note.ID }}', { method: 'POST' });
                if (!response.ok) throw new Error('Failed to share');
                const data = await response.json();
                this.shareToken = data.shareToken;
                this.shared = true;
                if (this.shareUrlConfigured) {
                    await updateClipboard(this.getShareUrl());
                }
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },
        async unshare() {
            this.loading = true;
            this.error = null;
            try {
                const response = await fetch('/v1/note/share?noteId={{ note.ID }}', { method: 'DELETE' });
                if (!response.ok) throw new Error('Failed to unshare');
                this.shareToken = '';
                this.shared = false;
            } catch (e) {
                this.error = e.message;
            } finally {
                this.loading = false;
            }
        },
        getShareUrl() {
            // BH-033: if SHARE_PUBLIC_URL is not configured, surface only the
            // relative path. Callers that check shareUrlConfigured first decide
            // whether to render this as an absolute URL or a "prepend your
            // public hostname" instruction.
            if (!this.shareUrlConfigured) {
                return '/s/' + this.shareToken;
            }
            return this.shareBaseUrl + '/s/' + this.shareToken;
        },
        async copyUrl() {
            if (!this.shareUrlConfigured) {
                return;
            }
            await updateClipboard(this.getShareUrl());
        }
    }">
        <template x-if="!shared">
            <button
                @click="share()"
                :disabled="loading"
                class="inline-flex items-center gap-2 px-3 py-1.5 text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800 rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
            >
                <svg x-show="!loading" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z"/>
                </svg>
                <svg x-show="loading" x-cloak class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Share Note
            </button>
        </template>
        <template x-if="shared">
            <div class="space-y-2">
                <div class="flex items-center gap-1">
                    <span class="inline-flex items-center px-2 py-0.5 bg-amber-100 text-amber-700 text-xs font-mono font-medium rounded">
                        Shared
                    </span>
                </div>
                {% if not shareUrlConfigured %}
                {# BH-033: warn the admin when SHARE_PUBLIC_URL is unset. The old fallback synthesized http://<bind-address>:<port> which is useless for any external recipient. #}
                <div class="p-2 bg-amber-50 border border-amber-200 rounded text-xs text-amber-800" data-testid="share-url-unconfigured-warning">
                    <p class="font-medium">Share URL base is not configured.</p>
                    <p class="mt-1">Set <code class="font-mono">SHARE_PUBLIC_URL</code> (flag: <code class="font-mono">--share-public-url=https://example.com</code>) to enable absolute shareable links. Until then, append the token path to your server's public URL manually.</p>
                </div>
                <div class="flex items-stretch gap-1">
                    <input
                        type="text"
                        :value="getShareUrl()"
                        readonly
                        aria-label="Relative share path"
                        class="flex-1 text-xs px-2 py-1 border border-stone-300 rounded-md bg-stone-50 text-stone-700 min-w-0 font-mono"
                    >
                </div>
                {% else %}
                <div class="flex items-stretch gap-1">
                    <input
                        type="text"
                        :value="getShareUrl()"
                        readonly
                        class="flex-1 text-xs px-2 py-1 border border-stone-300 rounded-l-md bg-stone-50 text-stone-700 min-w-0"
                    >
                    <button
                        @click="copyUrl()"
                        title="Copy URL"
                        class="px-2 py-1 text-stone-600 bg-stone-100 hover:bg-stone-200 border border-l-0 border-stone-300 rounded-r-md"
                    >
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3"/>
                        </svg>
                    </button>
                </div>
                {% endif %}
                <button
                    @click="unshare()"
                    :disabled="loading"
                    class="inline-flex items-center gap-1 px-2 py-1 text-xs font-mono font-medium text-red-700 hover:text-red-800 hover:bg-red-50 rounded disabled:opacity-50"
                >
                    <svg x-show="!loading" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636"/>
                    </svg>
                    <svg x-show="loading" x-cloak class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Unshare
                </button>
            </div>
        </template>
        <p x-show="error" x-cloak class="mt-1 text-xs text-red-700" x-text="error"></p>
    </div>
</div>
{% endif %}