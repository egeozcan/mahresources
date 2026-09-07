export const TIMELINE_PARAMS = ['timelineMode', 'timelineGranularity', 'timelineAnchor'];

export function readTimelineState(search, today = new Date().toISOString().slice(0, 10)) {
    const params = new URLSearchParams(search);
    const anchor = params.get('timelineAnchor') || '';
    const validDate = /^\d{4}-\d{2}-\d{2}$/.test(anchor)
        && !Number.isNaN(Date.parse(anchor))
        && new Date(anchor).toISOString().slice(0, 10) === anchor;
    return {
        timelineMode: params.get('timelineMode') === 'updated' ? 'updated' : 'created',
        granularity: ['week', 'month', 'year'].includes(params.get('timelineGranularity'))
            ? params.get('timelineGranularity') : 'month',
        anchor: validDate ? anchor : today,
    };
}

// Chart state belongs in the URL so saved searches and ordinary links agree.
// Mirror it into the native GET form as well; MRQL submissions already keep URL state.
export function writeTimelineState(state) {
    const params = new URLSearchParams(window.location.search);
    const values = [state.timelineMode, state.granularity, state.anchor];
    const form = document.querySelector('aside form[aria-label^="Filter"]');
    TIMELINE_PARAMS.forEach((name, i) => {
        params.set(name, values[i]);
        if (!form) return;
        let input = form.querySelector(`input[name="${name}"]`);
        if (!input) {
            input = document.createElement('input');
            input.type = 'hidden';
            input.name = name;
            form.append(input);
        }
        input.value = values[i];
    });
    window.history.replaceState(window.history.state, '', `${window.location.pathname}?${params}${window.location.hash}`);
}
