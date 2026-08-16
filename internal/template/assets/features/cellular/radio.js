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

function fillCATable(ca) {
    fillUplinkCASummary(ca);
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
