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
        lteCount: groups.LTE.length,
        nrCount: groups.NR.length,
        unknownCount: groups.Unknown.length
    };
}

function joinCarrierField(carriers, key) {
    const values = (carriers || []).map(c => c[key] || '—');
    return values.length ? values.join(' + ') : '—';
}

function fillUplinkCASummary(ca) {
    const ul = summarizeUplinkCA(ca);
    let status = 'Inactive';
    if (ul.active) {
        status = `Active (${ul.rat} ${ul.ccCount}CC)`;
    } else if (ul.lteCount || ul.nrCount || ul.unknownCount) {
        status = 'Single-carrier UL';
        if (ul.lteCount && ul.nrCount) status = 'EN-DC UL (not same-RAT CA)';
    }

    const counts = [];
    if (ul.lteCount) counts.push(`LTE ${ul.lteCount}CC`);
    if (ul.nrCount) counts.push(`NR ${ul.nrCount}CC`);
    if (ul.unknownCount) counts.push(`Unknown ${ul.unknownCount}CC`);

    setText('ul-ca-status', status);
    setText('ul-ca-carriers', counts.length ? counts.join(' · ') : '—');
    setText('ul-ca-bands', joinCarrierField(ul.carriers, 'band'));
    setText('ul-ca-mimo', joinCarrierField(ul.carriers, 'ul_mimo'));
    setText('ul-ca-modulation', joinCarrierField(ul.carriers, 'ul_modulation'));
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
        c.ul_active ? (c.component === 'PCC' ? 'primary' : 'UL CA') : '—',
        c.rsrp ? c.rsrp + ' dBm' : '—',
        c.rsrq ? c.rsrq + ' dB' : '—',
        c.sinr ? c.sinr + ' dB' : '—'
    ]), 15, 'No CA data yet');
}
