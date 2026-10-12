/* Location History 1.1 — Leaflet is shipped locally; queries stay in Lua. */
(() => {
    'use strict';
    const root = document.getElementById('lh-app');
    if (!root || !window.L) return;
    const $ = (selector) => root.querySelector(selector);
    const dayForm = $('#lh-day-form');
    const status = $('#lh-map-status');
    const dayInput = $('#lh-date');
    const nearPoint = $('#near-point');
    const radiusInput = $('#near-radius');
    const fixSelect = $('#lh-fix');
    const useFix = $('#lh-use-fix');
    const mapElement = $('#lh-map');
    dayForm.hidden = false;
    $('.lh-map-actions').hidden = false;
    mapElement.hidden = false;

    const map = L.map(mapElement, { scrollWheelZoom: false, preferCanvas: true }).setView([20, 0], 2);
    const historyLayer = L.featureGroup().addTo(map);
    const answerLayer = L.featureGroup().addTo(map);
    const selectionLayer = L.featureGroup().addTo(map);
    const tileNotice = document.createElement('p');
    tileNotice.className = 'lh-muted';
    tileNotice.hidden = true;
    tileNotice.textContent = 'Some map background tiles could not load. Recorded locations and search controls still work.';
    $('#lh-map-hint').after(tileNotice);
    let tileErrors = false;
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
        maxZoom: 19,
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).on('loading', () => { tileErrors = false; })
        .on('tileerror', () => { tileErrors = true; tileNotice.hidden = false; })
        .on('load', () => { tileNotice.hidden = !tileErrors; }).addTo(map);
    L.control.scale({ imperial: false }).addTo(map);

    let dayRequest, queryRequest;
    let dayGeneration = 0, queryGeneration = 0;
    let dayBounds;
    let dayPoints = [];
    const colors = { fix: '#2563eb', visit: '#047857', trip: '#b45309', estimate: '#7c3aed' };
    const coordinates = (lat, lng) => Number.isFinite(lat) && Number.isFinite(lng)
        && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180;
    const cancelDay = () => { dayGeneration++; dayRequest?.abort(); };
    const cancelQuery = () => {
        queryGeneration++;
        queryRequest?.abort();
        root.querySelectorAll('[data-lh-query]').forEach(form => {
            if (form.hasAttribute('aria-busy')) {
                $(`[data-lh-result="${form.dataset.lhQuery}"]`).textContent = 'Search cancelled. Submit your updated question to search again.';
            }
            form.removeAttribute('aria-busy');
        });
    };

    function choose(lat, lng, focus = false, notify = true) {
        // Leaflet can pan into wrapped copies of the world.
        lng = ((lng + 180) % 360 + 360) % 360 - 180;
        nearPoint.value = `${lat.toFixed(6)}, ${lng.toFixed(6)}`;
        if (notify) nearPoint.dispatchEvent(new Event('input', { bubbles: true }));
        else showSelection();
        if (focus) nearPoint.focus({ preventScroll: true });
    }

    function showSelection() {
        selectionLayer.clearLayers();
        const parts = nearPoint.value.replace(/^geo:/, '').split(',').map(value => value.replace('°', '').trim());
        if (parts.length !== 2 || parts.some(value => value === '')) return;
        const pair = parts.map(Number);
        if (!coordinates(...pair)) return;
        const radius = Number(radiusInput.value);
        if (Number.isFinite(radius) && radius > 0 && radius <= 100000) {
            L.circle(pair, { radius, color: colors.estimate, weight: 2, fillOpacity: 0.08 }).addTo(selectionLayer);
        }
        L.marker(pair, { title: 'Selected place', alt: 'Selected place' }).addTo(selectionLayer);
    }

    function popup(label, lat, lng) {
        const body = document.createElement('div');
        const text = document.createElement('p');
        text.textContent = label;
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = 'Search near this place';
        button.addEventListener('click', () => { choose(lat, lng, true); map.closePopup(); });
        body.append(text, button);
        return body;
    }

    function dot(layer, lat, lng, label, color, size = 5) {
        if (!coordinates(lat, lng)) return;
        L.circleMarker([lat, lng], { radius: size, color, fillColor: color, fillOpacity: 0.8, weight: 1 })
            .bindPopup(popup(label, lat, lng)).addTo(layer);
        if (layer === answerLayer) {
            appendPlace($('#lh-answer-places'), label, lat, lng);
            $('#lh-answer-locations').hidden = false;
        }
    }

    function appendPlace(list, label, lat, lng) {
        const li = document.createElement('li');
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = label;
        button.addEventListener('click', () => { choose(lat, lng, true); map.setView([lat, lng], 16); });
        li.append(button);
        list.append(li);
    }

    function clearAnswer() {
        answerLayer.clearLayers();
        $('#lh-answer-places').replaceChildren();
        $('#lh-answer-locations').hidden = true;
    }

    function fit(bounds) {
        if (bounds?.isValid()) map.fitBounds(bounds, { padding: [32, 32], maxZoom: 16, animate: false });
    }

    function showAnswer(result, kind, move = true) {
        clearAnswer();
        if (kind === 'near' && result.query) {
            choose(result.query.lat, result.query.lng, false, false);
            radiusInput.value = result.query.radius_m;
            showSelection();
            if (move) fit(selectionLayer.getBounds());
        } else if (kind === 'at') {
            const estimate = result.estimate;
            if (estimate) dot(answerLayer, estimate.lat, estimate.lng, `Estimated position: ${estimate.basis}`, colors.estimate, 10);
            for (const [name, point] of [['Last fix before', result.before], ['First fix after', result.after]]) {
                if (point) dot(answerLayer, point.lat, point.lng, `${name}: ${point.time}`, colors.fix, 7);
            }
            const segment = result.segment;
            if (segment) {
                dot(answerLayer, segment.lat, segment.lng,
                    `${segment.kind === 'visit' ? 'Visit' : 'Trip start'}: ${segment.place_name || segment.type || segment.kind}`,
                    segment.kind === 'visit' ? colors.visit : colors.trip, 8);
                dot(answerLayer, segment.end_lat, segment.end_lng, 'Trip destination', colors.trip, 8);
            }
            if (move) fit(answerLayer.getBounds());
        }
    }

    function clearDay() {
        historyLayer.clearLayers();
        $('#lh-places').replaceChildren();
        dayBounds = null;
        dayPoints = [];
        $('#lh-fit').disabled = true;
        $('#lh-fixes').hidden = true;
        fixSelect.replaceChildren(new Option('Choose a recorded fix', ''));
        useFix.disabled = true;
    }

    function showDay(data, move) {
        clearDay();
        dayPoints = data.points;
        for (const [index, [epoch, lat, lng, accuracy]] of data.points.entries()) {
            const time = new Date((epoch + data.offset_minutes * 60) * 1000).toISOString().slice(11, 19);
            dot(historyLayer, lat, lng, `Recorded fix at ${time} ${data.offset}${accuracy > 0 ? ` (±${accuracy} m)` : ''}`, colors.fix);
            fixSelect.add(new Option(`${time} ${data.offset} · ${lat.toFixed(6)}, ${lng.toFixed(6)}${accuracy > 0 ? ` (±${accuracy} m)` : ''}`, String(index)));
        }
        for (const segment of data.segments) {
            const label = `${segment.kind === 'visit' ? 'Visit' : 'Trip'}: ${segment.place_name || segment.type || segment.place_id || 'Unnamed place'} · ${segment.start} – ${segment.end} ${segment.offset}${segment.edited ? ' · edited by you' : ''}`;
            const endpoints = segment.kind === 'visit' ? [[segment.lat, segment.lng, '']]
                : [[segment.lat, segment.lng, ' · Start'], [segment.end_lat, segment.end_lng, ' · Destination']];
            for (const [lat, lng, suffix] of endpoints) {
                if (!coordinates(lat, lng)) continue;
                dot(historyLayer, lat, lng, label + suffix, segment.kind === 'visit' ? colors.visit : colors.trip, 8);
                appendPlace($('#lh-places'), label + suffix, lat, lng);
            }
        }
        dayBounds = historyLayer.getBounds();
        $('#lh-fixes').hidden = data.points.length === 0;
        $('#lh-fit').disabled = !dayBounds.isValid();
        status.textContent = data.point_count || data.segment_count
            ? `${data.point_count.toLocaleString()} recorded fixes · ${data.segment_count.toLocaleString()} visits and trips · ${data.offset}`
            : `No locations recorded for ${data.date} (${data.offset}). Try another day or import.`;
        if (data.truncated) status.textContent += ` Showing the first ${data.points.length.toLocaleString()} fixes and ${data.segments.length} visits and trips across the selected imports; this day's display is incomplete.`;
        if (move) fit(dayBounds);
    }

    async function loadDay(move = true) {
        cancelDay();
        const generation = dayGeneration;
        const question = queryGeneration;
        if (!dayInput.value) {
            status.textContent = $('#lh-resource').options.length > 1 ? 'Choose a day to explore your history.'
                : 'Nothing imported yet. Upload a Google Timeline JSON file and run Import location history to begin.';
            $('#lh-fit').disabled = true;
            return;
        }
        dayRequest = new AbortController();
        clearDay();
        status.textContent = 'Loading this day…';
        dayForm.setAttribute('aria-busy', 'true');
        try {
            const params = new URLSearchParams(new FormData(dayForm));
            const response = await fetch(`/v1/plugins/location-history/day?${params}`, { signal: dayRequest.signal, headers: { Accept: 'application/json' } });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || 'Could not load this day.');
            if (generation !== dayGeneration) return;
            showDay(data, move && question === queryGeneration);
        } catch (error) {
            if (generation === dayGeneration && error.name !== 'AbortError') status.textContent = `Could not load this day: ${error.message}. Try Show day again.`;
        } finally {
            if (generation === dayGeneration) dayForm.removeAttribute('aria-busy');
        }
    }

    async function runQuery(form, url) {
        cancelQuery();
        const generation = queryGeneration;
        queryRequest = new AbortController();
        const kind = form.dataset.lhQuery;
        const result = $(`[data-lh-result="${kind}"]`);
        form.setAttribute('aria-busy', 'true');
        result.textContent = 'Searching your history…';
        clearAnswer();
        try {
            const response = await fetch(url, { signal: queryRequest.signal, headers: { Accept: 'text/html' } });
            if (!response.ok) throw new Error(`Request failed (${response.status}).`);
            const document = new DOMParser().parseFromString(await response.text(), 'text/html');
            if (generation !== queryGeneration) return;
            const answer = document.getElementById(`${kind}-results`);
            if (!answer) throw new Error('The answer could not be loaded.');
            result.replaceChildren(answer);
            showAnswer(JSON.parse(answer.dataset.answer), kind);
            window.history.replaceState(null, '', url);
        } catch (error) {
            if (generation !== queryGeneration || error.name === 'AbortError') return;
            const message = document.createElement('p');
            message.setAttribute('role', 'alert');
            message.textContent = `Could not search your history: ${error.message} Try again.`;
            result.replaceChildren(message);
        } finally {
            if (generation === queryGeneration) form.removeAttribute('aria-busy');
        }
    }

    dayForm.addEventListener('submit', event => {
        event.preventDefault();
        cancelQuery();
        clearAnswer();
        loadDay();
    });
    dayForm.addEventListener('change', () => {
        cancelDay();
        dayForm.removeAttribute('aria-busy');
        cancelQuery();
        clearAnswer();
        if (dayForm.reportValidity()) loadDay();
        else {
            clearDay();
            status.textContent = 'Choose a valid day to explore your history.';
        }
    });
    for (const [id, delta] of [['lh-prev', -1], ['lh-next', 1]]) {
        $(`#${id}`).addEventListener('click', () => {
            if (!dayInput.value) return;
            const date = new Date(`${dayInput.value}T12:00:00Z`);
            date.setUTCDate(date.getUTCDate() + delta);
            const next = date.toISOString().slice(0, 10);
            if (next < dayInput.min || next > dayInput.max) return;
            dayInput.value = next;
            dayForm.dispatchEvent(new Event('change'));
        });
    }
    $('#lh-fit').addEventListener('click', () => fit(dayBounds));
    $('#lh-center').addEventListener('click', () => { const center = map.getCenter(); choose(center.lat, center.lng, true); });
    fixSelect.addEventListener('change', () => { useFix.disabled = fixSelect.value === ''; });
    useFix.addEventListener('click', () => {
        if (fixSelect.value === '') return;
        const [, lat, lng] = dayPoints[Number(fixSelect.value)];
        choose(lat, lng, true);
        map.setView([lat, lng], 16);
    });
    map.on('click', event => choose(event.latlng.lat, event.latlng.lng));
    nearPoint.addEventListener('input', showSelection);
    radiusInput.addEventListener('input', showSelection);
    root.querySelectorAll('[data-lh-query]').forEach(form => {
        form.addEventListener('submit', event => {
            event.preventDefault();
            const url = new URL(window.location.pathname, window.location.origin);
            url.search = new URLSearchParams(new FormData(form)).toString();
            runQuery(form, url);
        });
        // Edits invalidate a response for the old question even before resubmission.
        form.addEventListener('input', cancelQuery);
        form.addEventListener('change', cancelQuery);
    });
    root.addEventListener('click', event => {
        const link = event.target.closest('a');
        if (!link || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || event.button !== 0) return;
        const url = new URL(link.href, window.location.origin);
        if (url.origin !== window.location.origin || url.pathname !== window.location.pathname || url.searchParams.get('q') !== 'near') return;
        event.preventDefault();
        const form = $('[data-lh-query="near"]');
        for (const [key, fallback] of [['from', ''], ['to', ''], ['resource', ''], ['offset', 'auto']]) {
            form.elements.namedItem(key).value = url.searchParams.get(key) ?? fallback;
        }
        for (const [key, value] of url.searchParams) {
            const input = form.elements.namedItem(key);
            if (input) input.value = value;
        }
        showSelection();
        runQuery(form, url);
    });

    showSelection();
    const initial = $('[data-answer]');
    const initialAnswer = initial && JSON.parse(initial.dataset.answer);
    let initialBounds;
    if (initialAnswer) {
        const kind = initial.id === 'at-results' ? 'at' : 'near';
        showAnswer(initialAnswer, kind, false);
        initialBounds = kind === 'near' && initialAnswer.query ? selectionLayer.getBounds() : answerLayer.getBounds();
    }
    const hasInitialGeometry = initialBounds?.isValid();
    loadDay(!hasInitialGeometry).then(() => {
        if (hasInitialGeometry && queryGeneration === 0) fit(initialBounds);
    });
    window.addEventListener('pagehide', event => {
        cancelDay();
        cancelQuery();
        if (!event.persisted) map.remove();
    });
    window.addEventListener('pageshow', event => {
        if (event.persisted) { map.invalidateSize(); loadDay(false); }
    });
})();
