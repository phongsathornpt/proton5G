// Cellular feature overrides that depend on extended CA telemetry.
// Loaded after app.js so it replaces the base table renderer without adding a build step.
function caRAT(component) {
    const band = String((component && component.band) || '').trim().toLowerCase();
    if (band.startsWith('n')) return 'NR';
    if (band.startsWith('b')) return 'LTE';
    return 'Unknown';
}

function summarizeUplinkCA(ca) {
    const active = (ca || []).filter(c => c && c.ul_active);
    const groups = {LTE: [], NR: [], Unknown: []};

    active.forEach(c => {
        groups[caRAT(c)].push(c);
    });

    let rat = 'Unknown';
    let carriers = groups.Unknown;
    ['LTE', 'NR'].forEach(candidate => {
        if (groups[candidate].length > carriers.length ||
            (groups[candidate].length === carriers.length && rat === 'Unknown')) {
            rat = candidate;
            carriers = groups[candidate];
        }
    });

    return {
        active: (rat === 'LTE' || rat === 'NR') && carriers.length >= 2,
        rat,
        ccCount: carriers.length,
        carriers,
        allActive: active,
        lteCount: groups.LTE.length,
        nrCount: groups.NR.length,
        unknownCount: groups.Unknown.length
    };
}

function joinCarrierField(carriers, key) {
    const values = (carriers || []).map(c => c[key] || '—');
    return values.length ? values.join(' + ') : '—';
}

function initUplinkCAUI() {
    const status = document.getElementById('ul-ca-status');
    if (!status) return;

    const list = status.closest('.info-list');
    if (!list) return;

    list.classList.add('ul-ca-summary');
    list.setAttribute('aria-label', 'Uplink carrier aggregation summary');

    Array.from(list.querySelectorAll('.info-item')).forEach((item, index) => {
        item.classList.add('ul-ca-metric');
        if (index === 0) item.classList.add('ul-ca-metric-status');
    });

    status.classList.add('ul-ca-state', 'idle');
    status.setAttribute('aria-live', 'polite');
}

function setUplinkCAState(text, kind, title) {
    const el = document.getElementById('ul-ca-status');
    if (!el) return;

    el.textContent = text;
    el.className = `ul-ca-state ${kind || 'idle'}`;
    el.title = title || text;
}

function fillUplinkCASummary(ca) {
    const ul = summarizeUplinkCA(ca);
    let status = 'No UL CA';
    let statusKind = 'idle';
    let statusTitle = 'No uplink carrier aggregation is currently reported.';

    if (ul.active) {
        status = `UL ${ul.ccCount}CA active`;
        statusKind = 'ok';
        statusTitle = `${ul.rat} uplink carrier aggregation is active across ${ul.ccCount} component carriers.`;
    } else if (ul.lteCount && ul.nrCount) {
        status = 'EN-DC UL';
        statusKind = 'warn';
        statusTitle = 'LTE and NR uplink legs are active, but this is not same-RAT uplink carrier aggregation.';
    } else if (ul.lteCount || ul.nrCount || ul.unknownCount) {
        status = 'Single UL carrier';
        statusKind = 'idle';
        statusTitle = 'Only one same-RAT uplink carrier is currently active.';
    }

    const counts = [];
    if (ul.lteCount) counts.push(`LTE ${ul.lteCount}CC`);
    if (ul.nrCount) counts.push(`NR ${ul.nrCount}CC`);
    if (ul.unknownCount) counts.push(`Unknown ${ul.unknownCount}CC`);

    // When CA is active, show the winning same-RAT CA group. Otherwise show every
    // uplink-active carrier so EN-DC remains visible without being called 2CA.
    const displayCarriers = ul.active ? ul.carriers : ul.allActive;

    setUplinkCAState(status, statusKind, statusTitle);
    setText('ul-ca-carriers', counts.length ? counts.join(' · ') : '—');
    setText('ul-ca-bands', joinCarrierField(displayCarriers, 'band'));
    setText('ul-ca-mimo', joinCarrierField(displayCarriers, 'ul_mimo'));
    setText('ul-ca-modulation', joinCarrierField(displayCarriers, 'ul_modulation'));
}

function initDownlinkCapacityUI() {
    if (document.getElementById('dl-cap-estimate')) return;

    const panel = document.getElementById('panel-cellular');
    if (!panel) return;
    const grid = panel.querySelector('.grid');
    if (!grid) return;

    const card = document.createElement('section');
    card.className = 'card full-width dl-capacity-card';
    card.setAttribute('aria-label', 'Downlink capacity estimate');
    card.innerHTML = `
        <h2>Downlink capacity</h2>
        <div class="dl-capacity-hero">
            <div class="dl-capacity-primary">
                <span class="dl-capacity-eyebrow">Live radio ceiling estimate</span>
                <strong id="dl-cap-estimate">Waiting for radio data</strong>
                <span id="dl-cap-confidence" class="dl-cap-state idle" aria-live="polite">No CA telemetry</span>
            </div>
            <div class="dl-capacity-device">
                <span>FM350-GL device ceiling</span>
                <strong id="dl-cap-device">4.67 Gbps</strong>
            </div>
        </div>
        <div class="dl-capacity-grid">
            <div class="dl-capacity-metric"><span>DL carriers</span><strong id="dl-cap-carriers">—</strong></div>
            <div class="dl-capacity-metric"><span>Total bandwidth</span><strong id="dl-cap-bandwidth">—</strong></div>
            <div class="dl-capacity-metric"><span>LTE / NR bandwidth</span><strong id="dl-cap-rat-bandwidth">—</strong></div>
            <div class="dl-capacity-metric"><span>Bands</span><strong id="dl-cap-bands">—</strong></div>
            <div class="dl-capacity-metric"><span>Best DL MIMO</span><strong id="dl-cap-mimo">—</strong></div>
            <div class="dl-capacity-metric"><span>Best DL modulation</span><strong id="dl-cap-modulation">—</strong></div>
            <div class="dl-capacity-metric"><span>Estimated limiter</span><strong id="dl-cap-limiter">—</strong></div>
        </div>
        <p class="hint" id="dl-cap-note">Waiting for live AT+GTCAINFO carrier telemetry.</p>`;

    const cellsCard = document.getElementById('cells-table');
    const insertionPoint = cellsCard ? cellsCard.closest('.card') : null;
    if (insertionPoint) {
        grid.insertBefore(card, insertionPoint);
    } else {
        grid.appendChild(card);
    }
}

function formatCapacityMbps(mbps) {
    const n = Number(mbps || 0);
    if (!n) return '—';
    if (n >= 1000) return `${(n / 1000).toFixed(2)} Gbps`;
    return `${Math.round(n)} Mbps`;
}

function formatCapacityMHz(mhz) {
    const n = Number(mhz || 0);
    if (!n) return '—';
    return `${Number.isInteger(n) ? n : n.toFixed(1)} MHz`;
}

function setDownlinkCapacityState(text, kind, title) {
    const el = document.getElementById('dl-cap-confidence');
    if (!el) return;
    el.textContent = text;
    el.className = `dl-cap-state ${kind || 'idle'}`;
    el.title = title || text;
}

function fillDownlinkCapacity(capacity) {
    const c = capacity || {};
    const activeCC = Number(c.active_cc || 0);
    const estimatedCC = Number(c.estimated_from_cc || 0);
    const peak = Number(c.estimated_peak_mbps || 0);

    setText('dl-cap-estimate', peak ? formatCapacityMbps(peak) : 'Waiting for radio data');
    setText('dl-cap-device', formatCapacityMbps(c.device_ceiling_mbps || 4670));
    setText('dl-cap-carriers', activeCC ? `${activeCC} CC · LTE ${c.lte_cc || 0} · NR ${c.nr_cc || 0}` : '—');
    setText('dl-cap-bandwidth', formatCapacityMHz(c.total_bandwidth_mhz));
    setText('dl-cap-rat-bandwidth', `LTE ${formatCapacityMHz(c.lte_bandwidth_mhz)} · NR ${formatCapacityMHz(c.nr_bandwidth_mhz)}`);
    setText('dl-cap-bands', (c.bands || []).length ? c.bands.join(' + ') : '—');
    setText('dl-cap-mimo', c.best_dl_mimo || '—');
    setText('dl-cap-modulation', c.best_dl_modulation || '—');
    setText('dl-cap-limiter', c.limiter || '—');

    if (!activeCC) {
        setDownlinkCapacityState('No CA telemetry', 'idle', 'Waiting for AT+GTCAINFO carrier data.');
    } else if (!peak) {
        setDownlinkCapacityState('Estimate unavailable', 'warn', 'Bandwidth, MIMO, or modulation telemetry is missing for the reported carriers.');
    } else if (c.estimate_complete) {
        setDownlinkCapacityState('Live estimate complete', 'ok', `All ${activeCC} reported carriers contributed to the estimate.`);
    } else {
        setDownlinkCapacityState(`Partial · ${estimatedCC}/${activeCC} CC`, 'warn', 'Only carriers with known bandwidth, DL MIMO, and DL modulation contribute to the estimate.');
    }

    const note = document.getElementById('dl-cap-note');
    if (note) {
        const method = c.estimate_method || 'Live estimate uses reported bandwidth, DL MIMO, and modulation.';
        note.textContent = `${method}. This is a radio-side heuristic, not a guaranteed speed; SIM/package caps, TDD slots, scheduler load, RF quality, backhaul and protocol overhead can reduce real throughput.`;
    }
}

function fillCATable(ca) {
    fillUplinkCASummary(ca);
    const capacity = (typeof lastStatus !== 'undefined' && lastStatus) ? lastStatus.downlink_capacity : null;
    fillDownlinkCapacity(capacity);
    fillTableBody('ca-body', (ca || []).map(c => [
        c.component || '—',
        c.band || '—',
        c.pci || '—',
        c.arfcn || '—',
        c.ul_arfcn || '—',
        c.dl_bandwidth || '—',
        c.ul_bandwidth || '—',
        c.dl_mimo || '—',
        c.ul_mimo || '—',
        c.dl_modulation || '—',
        c.ul_modulation || '—',
        c.ul_active ? (c.component === 'PCC' ? 'Primary UL' : 'UL CA') : '—',
        c.rsrp ? c.rsrp + ' dBm' : '—',
        c.rsrq ? c.rsrq + ' dB' : '—',
        c.sinr ? c.sinr + ' dB' : '—'
    ]), 15, 'No CA data yet');
}

initUplinkCAUI();
initDownlinkCapacityUI();
fillDownlinkCapacity(null);
