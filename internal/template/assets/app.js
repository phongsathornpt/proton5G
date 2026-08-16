let evtSource = null;
let historyPoints = [];
let modemInventory = null;
let lastStatus = null;
let lastHotspot = null;
let dataSessionNote = '—';
let sseReconnectShown = false;

const API_TOKEN = (function () {
    try {
        const q = new URLSearchParams(location.search).get('token');
        if (q) {
            localStorage.setItem('fm350_token', q);
            return q;
        }
        return localStorage.getItem('fm350_token') || '';
    } catch (e) {
        return '';
    }
})();

function apiHeaders(json) {
    const h = {};
    if (json) h['Content-Type'] = 'application/json';
    if (API_TOKEN) h['X-API-Token'] = API_TOKEN;
    return h;
}

function apiURL(path) {
    if (!API_TOKEN) return path;
    const sep = path.indexOf('?') >= 0 ? '&' : '?';
    return path + sep + 'token=' + encodeURIComponent(API_TOKEN);
}

function toast(msg, kind) {
    const region = document.getElementById('toast-region');
    if (!region) {
        console.log(msg);
        return;
    }
    const el = document.createElement('div');
    el.className = 'toast' + (kind === 'err' ? ' err' : kind === 'ok' ? ' ok' : '');
    el.textContent = msg;
    region.appendChild(el);
    setTimeout(() => {
        el.remove();
    }, 4200);
}

async function withBusy(btn, fn) {
    if (btn) btn.disabled = true;
    try {
        return await fn();
    } finally {
        if (btn) btn.disabled = false;
    }
}

function setText(id, v) {
    const el = document.getElementById(id);
    if (el) el.innerText = v == null || v === '' ? '-' : String(v);
}

function setDot(id, state) {
    const el = document.getElementById(id);
    if (!el) return;
    el.className = 'dot ' + (state || 'disconnected');
}

function setChip(id, text, kind) {
    const el = document.getElementById(id);
    if (!el) return;
    el.textContent = text;
    el.className = 'chip' + (kind ? ' ' + kind : '');
}

/* Panel navigation lives in layout/layout.js (showPanel / initLayout). */

function markDirty(el) {
    if (el) el.dataset.dirty = '1';
}

function clearDirty(...ids) {
    ids.forEach(id => {
        const el = document.getElementById(id);
        if (el) delete el.dataset.dirty;
    });
}

function initDirtyTracking() {
    ['hotspot-ssid', 'hotspot-password', 'hotspot-channel', 'hotspot-wlan', 'apn-name', 'apn-type'].forEach(id => {
        const el = document.getElementById(id);
        if (!el) return;
        el.addEventListener('input', () => markDirty(el));
        el.addEventListener('change', () => markDirty(el));
    });
}

function initSSE() {
    initDirtyTracking();

    evtSource = new EventSource(apiURL('/api/events'));

    evtSource.onmessage = function (event) {
        try {
            sseReconnectShown = false;
            const data = JSON.parse(event.data);
            updateUI(data);
            if (data.signal && (data.signal.rssi || data.signal.percentage)) {
                historyPoints.push({
                    timestamp: new Date().toISOString(),
                    rssi: data.signal.rssi || 0,
                    percentage: data.signal.percentage || 0,
                    tech: (data.network && data.network.tech) || ''
                });
                if (historyPoints.length > 180) {
                    historyPoints = historyPoints.slice(-180);
                }
                drawSignalChart(historyPoints);
            }
        } catch (e) {
            console.error('SSE parse error', e);
        }
    };

    evtSource.onerror = function () {
        if (sseReconnectShown) return;
        sseReconnectShown = true;
        const cs = document.getElementById('conn-status');
        if (cs && !/SSE reconnecting/.test(cs.innerText || '')) {
            cs.innerText = (cs.innerText || 'AT') + ' · SSE reconnecting…';
        }
    };

    const modemSel = document.getElementById('modem-select');
    if (modemSel) {
        modemSel.addEventListener('change', onModemDropdownChange);
    }

    refreshHistory();
    refreshModems();
    refreshUSBMode();
    refreshHotspot();
    setInterval(refreshHistory, 15000);
    setInterval(refreshModems, 10000);
    setInterval(refreshUSBMode, 30000);
    setInterval(refreshHotspot, 10000);
}

function findModemById(id) {
    if (!modemInventory || !modemInventory.modems) return null;
    return modemInventory.modems.find(m => m.id === id) || null;
}

function fillSelect(sel, options, selected) {
    if (!sel) return;
    const prev = selected || sel.value;
    sel.innerHTML = '';
    if (!options.length) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = '(none found)';
        sel.appendChild(opt);
        sel.disabled = true;
        return;
    }
    sel.disabled = false;
    options.forEach(o => {
        const opt = document.createElement('option');
        opt.value = o.value;
        opt.textContent = o.label;
        sel.appendChild(opt);
    });
    if (prev && options.some(o => o.value === prev)) {
        sel.value = prev;
    }
}

function fillATAndDataForModem(modem) {
    const atOpts = (modem && modem.at_ports ? modem.at_ports : []).map(p => ({
        value: p.path,
        label: p.label || p.path
    }));
    let atSelected = modemInventory && modemInventory.selected_at_port;
    if (modem && atSelected && !atOpts.some(o => o.value === atSelected)) {
        atSelected = atOpts.length ? atOpts[0].value : '';
    }
    fillSelect(document.getElementById('at-port-select'), atOpts, atSelected);

    const dataOpts = [];
    (modem && modem.net_ifaces ? modem.net_ifaces : []).forEach(p => {
        dataOpts.push({
            value: 'rndis:' + p.path,
            label: p.label || (p.path + ' (RNDIS)'),
            mode: 'rndis',
            iface: p.path
        });
    });
    (modem && modem.mbim_nodes ? modem.mbim_nodes : []).forEach(p => {
        dataOpts.push({
            value: 'mbim:' + p.path,
            label: p.label || (p.path + ' (MBIM)'),
            mode: 'mbim',
            iface: p.path
        });
    });
    if (!dataOpts.length && modemInventory && modemInventory.modems) {
        modemInventory.modems.forEach(m => {
            (m.net_ifaces || []).forEach(p => {
                dataOpts.push({
                    value: 'rndis:' + p.path,
                    label: (m.name ? m.name + ' · ' : '') + (p.label || p.path),
                    mode: 'rndis',
                    iface: p.path
                });
            });
            (m.mbim_nodes || []).forEach(p => {
                dataOpts.push({
                    value: 'mbim:' + p.path,
                    label: (m.name ? m.name + ' · ' : '') + (p.label || p.path),
                    mode: 'mbim',
                    iface: p.path
                });
            });
        });
    }

    let dataSelected = '';
    if (modemInventory && modemInventory.selected_net) {
        dataSelected = 'rndis:' + modemInventory.selected_net;
    } else if (modemInventory && modemInventory.selected_mbim) {
        dataSelected = 'mbim:' + modemInventory.selected_mbim;
    }
    fillSelect(document.getElementById('data-iface-select'), dataOpts.map(o => ({
        value: o.value,
        label: o.label
    })), dataSelected);

    window.__dataIfaceOptions = dataOpts;

    const modeEl = document.getElementById('data-mode');
    if (modeEl) {
        modeEl.innerText = (modem && modem.data_mode) ? modem.data_mode : (dataOpts.length ? 'available' : 'none');
    }
    const btn = document.getElementById('data-connect-btn');
    if (btn) btn.disabled = dataOpts.length === 0;
    updateOverview();
}

function onModemDropdownChange() {
    const id = document.getElementById('modem-select').value;
    fillATAndDataForModem(findModemById(id));
}

function applyInventoryToUI(data) {
    modemInventory = data;
    const modemOpts = (data.modems || []).map(m => {
        const atN = (m.at_ports || []).length;
        const mbN = (m.mbim_nodes || []).length;
        const netN = (m.net_ifaces || []).length;
        let label = m.name || m.id;
        label += ` — ${atN} AT`;
        if (netN) label += `, ${netN} net`;
        if (mbN) label += `, ${mbN} MBIM`;
        return {value: m.id, label};
    });
    fillSelect(document.getElementById('modem-select'), modemOpts, data.selected_modem_id);
    const modem = findModemById(document.getElementById('modem-select').value) ||
        (data.modems && data.modems[0]) || null;
    fillATAndDataForModem(modem);

    const hint = document.getElementById('modem-hint');
    if (hint) {
        if (!(data.modems && data.modems.length)) {
            hint.textContent = 'No modem found. Check USB power / VID:PID and permissions.';
        } else if (data.note) {
            hint.textContent = data.note;
        }
    }
    const mbim = document.getElementById('mbim-cli');
    if (mbim) mbim.innerText = data.mbimcli_available ? 'yes' : 'no';
    const dh = document.getElementById('data-hint');
    if (dh && data.install_hint) {
        dh.textContent = data.install_hint;
    }
    updateOverview();
}

async function refreshModems() {
    try {
        const resp = await fetch(apiURL('/api/modems'), {headers: apiHeaders(false)});
        if (!resp.ok) return;
        applyInventoryToUI(await resp.json());
    } catch (e) {
        console.error(e);
    }
}

async function refreshUSBMode() {
    try {
        const resp = await fetch(apiURL('/api/usbmode'), {headers: apiHeaders(false)});
        if (!resp.ok) return;
        const data = await resp.json();
        setText('usb-mode-current', data.label || (data.mode ? String(data.mode) : '-') +
            (data.error ? ' (' + data.error + ')' : ''));
        const sel = document.getElementById('usb-mode-select');
        if (sel && data.supported) {
            const prev = sel.value;
            sel.innerHTML = '';
            data.supported.forEach(o => {
                const opt = document.createElement('option');
                opt.value = String(o.mode);
                opt.textContent = o.label || String(o.mode);
                sel.appendChild(opt);
            });
            if (data.mode) sel.value = String(data.mode);
            else if (prev) sel.value = prev;
        }
        const note = document.getElementById('usb-mode-hint');
        if (note && data.note) note.textContent = data.note;
    } catch (e) {
        // ignore
    }
}

async function applyUSBMode() {
    const sel = document.getElementById('usb-mode-select');
    const mode = sel ? parseInt(sel.value, 10) : 0;
    if (!mode) {
        toast('Pick a USB mode first', 'err');
        return;
    }
    if (!confirm('Set AT+GTUSBMODE=' + mode + '? Modem USB will re-enumerate (brief disconnect).')) return;
    try {
        const resp = await fetch(apiURL('/api/usbmode'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({mode: mode})
        });
        const res = await resp.json();
        toast(res.status === 'ok' ? 'USB mode applied' : (res.error || 'USB mode failed'),
            res.status === 'ok' ? 'ok' : 'err');
        logConsole('USB mode: ' + (res.status || res.error || '') + ' → ' +
            JSON.stringify(res.usb_mode || {}));
        setTimeout(() => { refreshUSBMode(); refreshModems(); }, 3000);
    } catch (e) {
        toast('USB mode error: ' + e, 'err');
        logConsole('USB mode error: ' + e);
    }
}

function parseDataIfaceSelection() {
    const val = (document.getElementById('data-iface-select') || {}).value || '';
    const opts = window.__dataIfaceOptions || [];
    const hit = opts.find(o => o.value === val);
    if (hit) return {mode: hit.mode, iface: hit.iface};
    if (val.startsWith('rndis:')) return {mode: 'rndis', iface: val.slice(6)};
    if (val.startsWith('mbim:')) return {mode: 'mbim', iface: val.slice(5)};
    return {mode: '', iface: val};
}

async function applyModemSelection() {
    const modem_id = document.getElementById('modem-select').value;
    const at_port = document.getElementById('at-port-select').value;
    const dataSel = parseDataIfaceSelection();
    const body = {
        modem_id,
        at_port,
        mbim_device: dataSel.mode === 'mbim' ? dataSel.iface : '',
        net_iface: dataSel.mode === 'rndis' ? dataSel.iface : ''
    };
    try {
        const resp = await fetch(apiURL('/api/modems/select'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify(body)
        });
        const data = await resp.json();
        if (!resp.ok) {
            toast(data.error || 'Select failed', 'err');
            logConsole(`Select modem: ${data.error || resp.status}`);
            return;
        }
        applyInventoryToUI(data);
        toast('Modem selection applied', 'ok');
        logConsole(`Using modem ${data.selected_modem_id || modem_id} AT=${data.selected_at_port || at_port || '-'} data=${data.selected_net || data.selected_mbim || '-'}`);
        const st = await fetch(apiURL('/api/status'), {headers: apiHeaders(false)});
        if (st.ok) updateUI(await st.json());
    } catch (e) {
        toast('Select error: ' + e, 'err');
        logConsole(`Select modem error: ${e}`);
    }
}

async function dataConnect() {
    const apn = (document.getElementById('apn-name') || {}).value || '';
    const sel = parseDataIfaceSelection();
    if (!sel.iface) {
        toast('No data interface — modem may only expose AT', 'err');
        return;
    }
    const btn = document.getElementById('data-connect-btn');
    await withBusy(btn, async () => {
        try {
            await applyModemSelection();
            const methodSel = document.getElementById('wan-method-select');
            const method = (methodSel && methodSel.value) || 'auto';
            const resp = await fetch(apiURL('/api/data/connect'), {
                method: 'POST',
                headers: apiHeaders(true),
                body: JSON.stringify({mode: sel.mode, iface: sel.iface, apn: apn, method: method})
            });
            const res = await resp.json();
            const ok = resp.ok && !res.error;
            dataSessionNote = ok ? `connected (${sel.mode} ${sel.iface})` : (res.error || 'failed');
            setText('data-status', dataSessionNote);
            toast(ok ? 'WAN connect OK' : ('WAN: ' + (res.error || 'failed')), ok ? 'ok' : 'err');
            logConsole(`Data connect (${sel.mode} ${sel.iface}): ${res.status || res.error || ''}\n${res.output || ''}`);
            refreshModems();
            refreshHotspot();
            updateOverview();
        } catch (e) {
            dataSessionNote = 'error';
            setText('data-status', dataSessionNote);
            toast('Data connect error: ' + e, 'err');
            logConsole(`Data connect error: ${e}`);
        }
    });
}

async function dataDisconnect() {
    const sel = parseDataIfaceSelection();
    try {
        const resp = await fetch(apiURL('/api/data/disconnect'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({mode: sel.mode, iface: sel.iface})
        });
        const res = await resp.json();
        dataSessionNote = 'disconnected';
        setText('data-status', dataSessionNote);
        toast('WAN disconnected', 'ok');
        logConsole(`Data disconnect (${sel.mode} ${sel.iface}): ${res.status || res.error || ''}\n${res.output || ''}`);
        refreshModems();
        updateOverview();
    } catch (e) {
        toast('Disconnect error: ' + e, 'err');
        logConsole(`Data disconnect error: ${e}`);
    }
}

function fmtBytes(n) {
    n = Number(n) || 0;
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
        n /= 1024;
        i++;
    }
    return n.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function fmtRate(bps) {
    if (bps == null || bps === '' || Number(bps) < 0) return '-';
    return fmtBytes(Number(bps)) + '/s';
}

function fillTableBody(bodyId, rows, emptyCols, emptyText) {
    const body = document.getElementById(bodyId);
    if (!body) return;
    body.innerHTML = '';
    if (!rows || !rows.length) {
        const tr = document.createElement('tr');
        const td = document.createElement('td');
        td.colSpan = emptyCols;
        td.className = 'muted';
        td.textContent = emptyText;
        tr.appendChild(td);
        body.appendChild(tr);
        return;
    }
    rows.forEach(cols => {
        const tr = document.createElement('tr');
        cols.forEach(v => {
            const td = document.createElement('td');
            td.textContent = v == null || v === '' ? '—' : String(v);
            tr.appendChild(td);
        });
        body.appendChild(tr);
    });
}

function fillCellsTable(cells) {
    fillTableBody('cells-body', (cells || []).map(c => [
        c.serving ? 'Yes' : 'No',
        c.rat || '—',
        c.cell_id || '—',
        c.pci || '—',
        c.band || '—',
        c.arfcn || '—',
        c.rsrp ? c.rsrp + ' dBm' : '—',
        c.rsrq ? c.rsrq + ' dB' : '—',
        c.sinr ? c.sinr + ' dB' : '—'
    ]), 9, 'No cell data yet');
}

function fillCATable(ca) {
    fillTableBody('ca-body', (ca || []).map(c => [
        c.component || '—',
        c.band || '—',
        c.pci || '—',
        c.arfcn || '—',
        c.ul_arfcn || '—',
        c.dl_bandwidth || '—',
        c.ul_bandwidth || '—',
        c.dl_modulation || '—',
        c.ul_modulation || '—',
        c.ul_active ? 'Active' : (c.component === 'PCC' ? 'Active' : '—'),
        c.rsrp ? c.rsrp + ' dBm' : '—',
        c.rsrq ? c.rsrq + ' dB' : '—',
        c.sinr ? c.sinr + ' dB' : '—'
    ]), 13, 'No CA data yet');
}

function fillWANDetails(data) {
    const pdp = data.pdp || {};
    setText('pdp-ip', pdp.ip || '-');
    const dns = [pdp.dns1, pdp.dns2].filter(Boolean).join(', ');
    setText('pdp-dns', dns || '-');
    setText('pdp-gateway', pdp.gateway || '-');
    const wan = data.wan || {};
    setText('wan-method', wan.method || '-');
    if (wan.session && wan.session !== 'disconnected') {
        setText('data-status', wan.session + (wan.method ? ' (' + wan.method + ')' : ''));
    }
    if (wan.addrs && wan.addrs.length) {
        setText('wan-ip', wan.addrs[0]);
    } else if (pdp.ip) {
        setText('wan-ip', pdp.ip);
    }
    setText('wan-rx', wan.rx_bytes ? fmtBytes(wan.rx_bytes) : '-');
    setText('wan-tx', wan.tx_bytes ? fmtBytes(wan.tx_bytes) : '-');
    setText('wan-rx-rate', wan.rx_rate_bps ? fmtRate(wan.rx_rate_bps) : '-');
    setText('wan-tx-rate', wan.tx_rate_bps ? fmtRate(wan.tx_rate_bps) : '-');
}

function updateHeaderStatus(data) {
    const atOk = data && data.modem && data.modem.connected;
    setDot('status-at-dot', atOk ? 'ok' : 'err');
    setDot('conn-dot', atOk ? 'connected' : 'disconnected');
    const port = (data && data.modem && data.modem.port_path) || '';
    setText('status-at-text', atOk ? (port || 'up') : 'down');
    setText('conn-status', atOk ? `AT linked (${port || 'USB'})` : 'AT disconnected');

    const wanAddrs = lastHotspot && lastHotspot.uplink_addrs;
    const wanIface = (lastHotspot && lastHotspot.uplink_iface) ||
        (modemInventory && modemInventory.selected_net) || '';
    const wanUp = wanAddrs && wanAddrs.length;
    setDot('status-wan-dot', wanUp ? 'ok' : (wanIface ? 'warn' : 'err'));
    setText('status-wan-text', wanUp ? wanAddrs[0] : (wanIface || 'no iface'));

    const apState = (lastHotspot && lastHotspot.state) || 'stopped';
    const apRun = apState === 'running';
    setDot('status-ap-dot', apRun ? 'ok' : (apState === 'error' ? 'err' : 'disconnected'));
    setText('status-ap-text', apState);
}

function updateOverview() {
    const data = lastStatus || {};
    const atOk = data.modem && data.modem.connected;
    setChip('ov-chip-at', 'AT: ' + (atOk ? 'linked' : 'down'), atOk ? 'ok' : 'err');
    const sim = (data.sim && data.sim.state) || '—';
    const simOk = /ready/i.test(sim);
    setChip('ov-chip-sim', 'SIM: ' + sim, simOk ? 'ok' : 'warn');
    const reg = (data.network && data.network.reg_state) || '—';
    const regOk = /home|roam|registered/i.test(reg);
    setChip('ov-chip-reg', 'Reg: ' + reg, regOk ? 'ok' : 'warn');

    const wanAddrs = lastHotspot && lastHotspot.uplink_addrs;
    const wanUp = wanAddrs && wanAddrs.length;
    setChip('ov-chip-wan', 'WAN: ' + (wanUp ? 'up' : 'down'), wanUp ? 'ok' : 'err');
    const apState = (lastHotspot && lastHotspot.state) || 'stopped';
    setChip('ov-chip-ap', 'AP: ' + apState, apState === 'running' ? 'ok' : (apState === 'error' ? 'err' : ''));

    const pct = (data.signal && data.signal.percentage) || 0;
    const fill = document.getElementById('ov-signal-fill');
    if (fill) fill.style.width = pct + '%';
    setText('ov-signal-pct', pct + '%');
    const op = (data.network && data.network.operator) || '-';
    const tech = (data.network && data.network.tech) || '-';
    setText('ov-net', op + ' · ' + tech);
    const rsrp = data.signal && data.signal.rsrp ? data.signal.rsrp + ' dBm' : '—';
    const sinr = data.signal && data.signal.sinr ? data.signal.sinr + ' dB' : '—';
    setText('ov-sig-detail', 'RSRP ' + rsrp + ' · SINR ' + sinr);
    if (data.temperature_c) {
        setText('ov-temp', data.temperature_c.toFixed(1) + ' °C');
    } else {
        setText('ov-temp', '-');
    }

    let updated = '-';
    if (data.updated_at) {
        try {
            updated = new Date(data.updated_at).toLocaleString();
        } catch (e) {
            updated = String(data.updated_at);
        }
    }
    setText('ov-updated', updated);
    setText('footer-updated', updated);

    const iface = (lastHotspot && lastHotspot.uplink_iface) ||
        (modemInventory && modemInventory.selected_net) || '-';
    const ips = wanAddrs && wanAddrs.length ? wanAddrs.join(', ') : '-';
    setText('ov-wan-detail', iface + ' · ' + ips);
    const pdpIP = data.pdp && data.pdp.ip;
    setText('wan-ip', (data.apn && data.apn.ip_addr) || pdpIP || ips);

    const ssid = lastHotspot && lastHotspot.config && lastHotspot.config.ssid;
    const ncli = (lastHotspot && lastHotspot.clients && lastHotspot.clients.length) || 0;
    setText('ov-ap-detail', (ssid || '—') + ' · ' + apState + ' · ' + ncli + ' client(s)');

    if (dataSessionNote && dataSessionNote !== '—') {
        setText('data-status', dataSessionNote);
    } else if (data.wan && data.wan.session) {
        setText('data-status', data.wan.session + (data.wan.method ? ' (' + data.wan.method + ')' : ''));
    } else {
        setText('data-status', '—');
    }
    updateHeaderStatus(data);
}

function updateUI(data) {
    lastStatus = data;
    updateHeaderStatus(data);

    if (data.modem) {
        setText('modem-port', data.modem.port_path || '-');
        setText('modem-power', data.modem.power_control || '-');
    }

    if (data.signal) {
        const pct = data.signal.percentage || 0;
        const sf = document.getElementById('signal-fill');
        if (sf) sf.style.width = pct + '%';
        setText('signal-pct', pct + '%');
        setText('sig-rssi', data.signal.rssi ? `${data.signal.rssi} dBm` : '-');
        setText('sig-rsrp', data.signal.rsrp ? `${data.signal.rsrp} dBm` : '-');
        setText('sig-rsrq', data.signal.rsrq ? `${data.signal.rsrq} dB` : '-');
        setText('sig-sinr', data.signal.sinr ? `${data.signal.sinr} dB` : '-');
    }

    if (data.temperature_c) {
        setText('modem-temp', Number(data.temperature_c).toFixed(1) + ' °C');
    } else {
        setText('modem-temp', '-');
    }

    if (data.identity) {
        setText('ident-manufacturer', data.identity.manufacturer || '-');
        setText('ident-model', data.identity.model || '-');
        setText('ident-firmware', data.identity.firmware || '-');
        setText('ident-imei', data.identity.imei || '-');
    }

    fillCellsTable(data.cells || []);
    fillCATable(data.ca || []);
    fillWANDetails(data);

    if (data.network) {
        setText('net-operator', data.network.operator || '-');
        setText('net-tech', data.network.tech || '-');
        setText('net-reg', data.network.reg_state || '-');
    }

    if (data.sim) {
        setText('sim-state', data.sim.state || '-');
        setText('sim-imsi', data.sim.imsi || '-');
        setText('sim-iccid', data.sim.iccid || '-');
    }

    if (data.rat_mode) {
        setText('rat-mode', data.rat_mode);
    }

    if (data.apn && data.apn.apn) {
        const apnInput = document.getElementById('apn-name');
        if (apnInput && document.activeElement !== apnInput && !apnInput.dataset.dirty) {
            apnInput.value = data.apn.apn;
        }
        const apnType = document.getElementById('apn-type');
        if (data.apn.pdp_type && apnType && !apnType.dataset.dirty) {
            apnType.value = data.apn.pdp_type;
        }
        if (data.apn.ip_addr) {
            setText('wan-ip', data.apn.ip_addr);
        }
    }

    const errEl = document.getElementById('status-error');
    if (errEl) errEl.innerText = data.error || '';

    updateOverview();
}

async function setRAT(mode) {
    try {
        const resp = await fetch(apiURL('/api/rat'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({mode: mode})
        });
        const res = await resp.json();
        toast(res.status === 'ok' ? 'RAT set to ' + mode : (res.error || 'RAT failed'),
            res.status === 'ok' ? 'ok' : 'err');
        logConsole(`Set RAT mode (${mode}): ${res.status || res.error}`);
    } catch (e) {
        toast('RAT error: ' + e, 'err');
        logConsole(`Error setting RAT mode: ${e}`);
    }
}

async function updateAPN(event) {
    event.preventDefault();
    const type = document.getElementById('apn-type').value;
    const name = document.getElementById('apn-name').value;
    try {
        const resp = await fetch(apiURL('/api/apn'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({cid: 1, pdp_type: type, apn: name})
        });
        const res = await resp.json();
        const ok = resp.ok && !res.error;
        toast(ok ? 'APN applied' : (res.error || 'APN failed'), ok ? 'ok' : 'err');
        if (ok) clearDirty('apn-name', 'apn-type');
        logConsole(`Update APN (${name}): ${res.status || res.error}`);
    } catch (e) {
        toast('APN error: ' + e, 'err');
        logConsole(`Error updating APN: ${e}`);
    }
}

async function sendRawAT(event) {
    event.preventDefault();
    const input = document.getElementById('at-input');
    const cmd = input.value.trim();
    if (!cmd) return;

    logConsole(`> ${cmd}`);
    input.value = '';

    try {
        const resp = await fetch(apiURL('/api/raw'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({command: cmd})
        });
        const res = await resp.json();
        logConsole(res.response || res.error || 'No response');
    } catch (e) {
        logConsole(`Error sending AT: ${e}`);
    }
}

async function usbReset() {
    if (!confirm('Issue USBDEVFS_RESET to the modem?')) return;
    try {
        const resp = await fetch(apiURL('/api/reset'), {
            method: 'POST',
            headers: apiHeaders(false)
        });
        const res = await resp.json();
        toast(res.status === 'ok' ? 'USB reset issued' : (res.error || 'reset failed'),
            res.status === 'ok' ? 'ok' : 'err');
        logConsole(`USB reset: ${res.status || res.error}`);
    } catch (e) {
        toast('USB reset error: ' + e, 'err');
        logConsole(`USB reset error: ${e}`);
    }
}

async function refreshHistory() {
    try {
        const resp = await fetch(apiURL('/api/history'), {headers: apiHeaders(false)});
        const data = await resp.json();
        if (Array.isArray(data) && data.length) {
            historyPoints = data;
            drawSignalChart(historyPoints);
        }
    } catch (e) {
        // ignore
    }
}

function drawSignalChart(points) {
    const canvas = document.getElementById('signal-chart');
    if (!canvas || !points || !points.length) return;

    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const cssW = canvas.clientWidth || 900;
    const cssH = 140;
    canvas.width = Math.floor(cssW * dpr);
    canvas.height = Math.floor(cssH * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    const w = cssW;
    const h = cssH;
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = '#090d16';
    ctx.fillRect(0, 0, w, h);

    const pad = 12;
    const vals = points.map(p => (typeof p.percentage === 'number' ? p.percentage : 0));
    const minV = 0;
    const maxV = 100;

    ctx.strokeStyle = '#1e293b';
    ctx.lineWidth = 1;
    for (let y = 0; y <= 4; y++) {
        const yy = pad + ((h - 2 * pad) * y) / 4;
        ctx.beginPath();
        ctx.moveTo(pad, yy);
        ctx.lineTo(w - pad, yy);
        ctx.stroke();
    }

    ctx.strokeStyle = '#38bdf8';
    ctx.lineWidth = 2;
    ctx.beginPath();
    vals.forEach((v, i) => {
        const x = pad + (i * (w - 2 * pad)) / Math.max(vals.length - 1, 1);
        const y = h - pad - ((v - minV) / (maxV - minV)) * (h - 2 * pad);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
    });
    ctx.stroke();

    const last = vals[vals.length - 1];
    ctx.fillStyle = '#94a3b8';
    ctx.font = '12px system-ui, sans-serif';
    ctx.fillText(`${last}% · ${vals.length} samples`, pad, 14);
}

function logConsole(msg) {
    const consoleDiv = document.getElementById('console-output');
    if (!consoleDiv) return;
    consoleDiv.innerText += `\n${msg}`;
    consoleDiv.scrollTop = consoleDiv.scrollHeight;
}

function fillClientTable(clients, running) {
    const body = document.getElementById('hotspot-client-body');
    if (!body) return;
    body.innerHTML = '';
    if (!clients || !clients.length) {
        const tr = document.createElement('tr');
        const td = document.createElement('td');
        td.colSpan = 3;
        td.className = 'muted';
        td.textContent = running ? 'No clients associated' : 'Hotspot not running';
        tr.appendChild(td);
        body.appendChild(tr);
        return;
    }
    clients.forEach(c => {
        const tr = document.createElement('tr');
        [c.mac || '—', c.ip || '—', c.name || '—'].forEach(v => {
            const td = document.createElement('td');
            td.textContent = v;
            tr.appendChild(td);
        });
        body.appendChild(tr);
    });
}

function refreshHotspot() {
    return fetch(apiURL('/api/hotspot'), {headers: apiHeaders(false)})
        .then(r => r.json())
        .then(st => {
            lastHotspot = st;
            setText('hotspot-state', st.state || '-');
            const chip = document.getElementById('hotspot-state-chip');
            if (chip) {
                chip.textContent = 'State: ' + (st.state || '—');
                chip.className = 'chip' + (st.state === 'running' ? ' ok' : st.state === 'error' ? ' err' : '');
            }
            const up = (st.uplink_iface || '-') + (st.uplink_addrs && st.uplink_addrs.length
                ? ' (' + st.uplink_addrs.join(', ') + ')' : '');
            setText('hotspot-uplink', up);
            setText('hotspot-lan', (st.lan_addrs && st.lan_addrs.length)
                ? st.lan_addrs.join(', ')
                : ((st.config && st.config.lan_cidr) || '-'));
            const t = st.tools || {};
            const missing = [];
            if (!t.hostapd) missing.push('hostapd');
            if (!t.dnsmasq) missing.push('dnsmasq');
            if (!t.iw) missing.push('iw');
            if (!t.ip) missing.push('ip');
            if (!t.nftables && !t.iptables) missing.push('nft|iptables');
            setText('hotspot-tools', missing.length ? 'missing: ' + missing.join(', ') : 'ok');

            const clients = st.clients || [];
            if (!clients.length) {
                setText('hotspot-clients', st.state === 'running' ? '0' : '-');
            } else {
                setText('hotspot-clients', String(clients.length));
            }
            fillClientTable(clients, st.state === 'running');

            const sel = document.getElementById('hotspot-wlan');
            if (sel && !sel.dataset.dirty) {
                const cur = sel.value;
                sel.innerHTML = '';
                const devs = st.devices || [];
                if (!devs.length) {
                    const o = document.createElement('option');
                    o.value = '';
                    o.textContent = '(no wireless ifaces)';
                    sel.appendChild(o);
                } else {
                    devs.forEach(d => {
                        const o = document.createElement('option');
                        o.value = d.iface;
                        o.textContent = d.label || d.iface;
                        // Only disable when AP is known unsupported (not when iw missing).
                        o.disabled = !!(d.ap_known && !d.supports_ap);
                        sel.appendChild(o);
                    });
                }
                const prefer = (st.config && st.config.wlan_iface) || cur;
                if (prefer) sel.value = prefer;
            }
            if (st.config) {
                const ssid = document.getElementById('hotspot-ssid');
                if (ssid && !ssid.dataset.dirty) ssid.value = st.config.ssid || '';
                const ch = document.getElementById('hotspot-channel');
                if (ch && !ch.dataset.dirty) ch.value = st.config.channel || 6;
            }
            const hint = document.getElementById('hotspot-hint');
            if (hint) {
                if (st.error) {
                    hint.textContent = st.error + (st.note ? ' — ' + st.note : '');
                } else if (st.install_hint) {
                    hint.textContent = 'Missing tools: ' + st.install_hint + (st.note ? ' — ' + st.note : '');
                } else if (st.note) {
                    hint.textContent = st.note;
                }
            }

            const startBtn = document.getElementById('hotspot-start-btn');
            if (startBtn) {
                const toolsOk = t.hostapd && t.dnsmasq && t.ip && (t.nftables || t.iptables);
                const hasUplink = !!(st.uplink_addrs && st.uplink_addrs.length);
                startBtn.disabled = !toolsOk || st.state === 'running';
                const tips = [];
                if (!toolsOk) tips.push(st.install_hint || 'install hostapd dnsmasq iw');
                if (!hasUplink) tips.push('connect WAN/RNDIS for IPv4 uplink');
                startBtn.title = tips.join('; ') || 'Start software AP';
            }

            renderHotspotDiag(st);

            updateOverview();
        })
        .catch(e => console.error('hotspot status', e));
}

function renderHotspotDiag(st) {
    const body = document.getElementById('hotspot-diag-body');
    const hintEl = document.getElementById('hotspot-install-hint');
    if (hintEl) {
        hintEl.textContent = st.install_hint
            ? st.install_hint
            : (st.diagnostics && st.diagnostics.notes && st.diagnostics.notes.length
                ? st.diagnostics.notes.join(' · ')
                : 'Tools look ready.');
    }
    if (!body) return;
    const devs = (st.diagnostics && st.diagnostics.interfaces) || st.devices || [];
    body.innerHTML = '';
    if (!devs.length) {
        const tr = document.createElement('tr');
        const td = document.createElement('td');
        td.colSpan = 5;
        td.className = 'muted';
        td.textContent = 'No wireless interfaces (check kernel/driver)';
        tr.appendChild(td);
        body.appendChild(tr);
        return;
    }
    devs.forEach(d => {
        const tr = document.createElement('tr');
        const ap = d.ap_known ? (d.supports_ap ? 'yes' : 'no') : 'unknown';
        [d.iface || '—', d.driver || '—', d.oper_state || '—', ap, d.phy || '—'].forEach(v => {
            const td = document.createElement('td');
            td.textContent = v;
            tr.appendChild(td);
        });
        body.appendChild(tr);
    });
}

function hotspotFormBody() {
    const body = {
        ssid: (document.getElementById('hotspot-ssid') || {}).value || '',
        wlan_iface: (document.getElementById('hotspot-wlan') || {}).value || '',
        channel: parseInt((document.getElementById('hotspot-channel') || {}).value || '6', 10),
        band: '2.4',
        lan_cidr: '192.168.50.1/24'
    };
    const pass = (document.getElementById('hotspot-password') || {}).value || '';
    if (pass) body.password = pass;
    return body;
}

function hotspotSaveConfig() {
    return fetch(apiURL('/api/hotspot/config'), {
        method: 'POST',
        headers: apiHeaders(true),
        body: JSON.stringify(hotspotFormBody())
    }).then(async r => {
        const j = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(j.error || r.statusText);
        clearDirty('hotspot-ssid', 'hotspot-password', 'hotspot-channel', 'hotspot-wlan');
        toast('Hotspot config saved', 'ok');
        return refreshHotspot();
    }).catch(e => toast('Save failed: ' + e.message, 'err'));
}

function hotspotStart() {
    const body = hotspotFormBody();
    if (!body.password) delete body.password;
    const btn = document.getElementById('hotspot-start-btn');
    return withBusy(btn, () =>
        fetch(apiURL('/api/hotspot/start'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify(body)
        }).then(async r => {
            const j = await r.json().catch(() => ({}));
            if (!r.ok) throw new Error(j.error || r.statusText);
            clearDirty('hotspot-ssid', 'hotspot-password', 'hotspot-channel', 'hotspot-wlan');
            toast('Hotspot started', 'ok');
            return refreshHotspot();
        }).catch(e => toast('Hotspot start failed: ' + e.message, 'err'))
    );
}

function hotspotStop() {
    const btn = document.getElementById('hotspot-stop-btn');
    return withBusy(btn, () =>
        fetch(apiURL('/api/hotspot/stop'), {
            method: 'POST',
            headers: apiHeaders(true)
        }).then(async r => {
            const j = await r.json().catch(() => ({}));
            if (!r.ok) throw new Error(j.error || r.statusText);
            toast('Hotspot stopped', 'ok');
            return refreshHotspot();
        }).catch(e => toast('Hotspot stop failed: ' + e.message, 'err'))
    );
}

// Bootstrapped by assets/boot.js (initLayout + initSSE).
window.addEventListener('resize', () => drawSignalChart(historyPoints));
window.historyPoints = historyPoints;
// Keep historyPoints in sync for layout.js chart redraw
Object.defineProperty(window, 'historyPointsRef', {
    get: function () { return historyPoints; }
});
