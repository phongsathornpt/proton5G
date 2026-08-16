// Cellular feature overrides that depend on extended CA telemetry.
// Loaded after app.js so it replaces the base table renderer without adding a build step.
function fillCATable(ca) {
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
        c.ul_active ? 'yes' : '—',
        c.rsrp ? c.rsrp + ' dBm' : '—',
        c.rsrq ? c.rsrq + ' dB' : '—',
        c.sinr ? c.sinr + ' dB' : '—'
    ]), 15, 'No CA data yet');
}
