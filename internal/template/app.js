let evtSource = null;
let historyPoints = [];
let modemInventory = null;

// Optional API token: ?token=… or localStorage fm350_token (for -token / FM350_API_TOKEN).
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

function initSSE() {
    evtSource = new EventSource(apiURL('/api/events'));

    evtSource.onmessage = function(event) {
        try {
            const data = JSON.parse(event.data);
            updateUI(data);
            // Keep history chart in sync cheaply from SSE signal samples
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
            console.error("SSE parse error", e);
        }
    };

    evtSource.onerror = function() {
        // SSE transport issue — do not claim modem USB is down
        document.getElementById('conn-status').innerText =
            (document.getElementById('conn-status').innerText || '') + ' (SSE reconnecting…)';
    };

    const modemSel = document.getElementById('modem-select');
    if (modemSel) {
        modemSel.addEventListener('change', onModemDropdownChange);
    }

    refreshHistory();
    refreshModems();
    refreshUSBMode();
    setInterval(refreshHistory, 15000);
    setInterval(refreshModems, 10000);
    setInterval(refreshUSBMode, 30000);
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

    // Data interfaces: prefer RNDIS net ifaces, else MBIM nodes
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
    // Fallback: collect from all modems if current has none
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

    // stash full option meta for connect
    window.__dataIfaceOptions = dataOpts;

    const modeEl = document.getElementById('data-mode');
    if (modeEl) {
        modeEl.innerText = (modem && modem.data_mode) ? modem.data_mode : (dataOpts.length ? 'available' : 'none');
    }
    const btn = document.getElementById('data-connect-btn');
    if (btn) btn.disabled = dataOpts.length === 0;
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
            hint.textContent = 'No modems found. Plug in the FM350 and click Refresh (run as root if permission denied).';
        } else if (data.note) {
            hint.textContent = data.note;
        } else {
            hint.textContent = 'Select a modem and AT port, then Use selected to connect monitoring.';
        }
    }

    document.getElementById('mbim-cli').innerText = data.mbimcli_available ? 'available' : 'missing';
    const dataHint = document.getElementById('data-hint');
    if (dataHint) {
        if (data.note) {
            dataHint.textContent = data.note;
        } else {
            dataHint.innerHTML = 'Apply APN via AT first. <strong>RNDIS</strong>: DHCP on net iface. <strong>MBIM</strong>: needs <code>/dev/cdc-wdm*</code> + mbimcli.';
        }
    }
}

async function refreshModems() {
    try {
        const resp = await fetch(apiURL('/api/modems'), {headers: apiHeaders(false)});
        if (resp.status === 401) {
            logConsole('Unauthorized — open UI with ?token=YOUR_TOKEN or set localStorage fm350_token');
            return;
        }
        const data = await resp.json();
        applyInventoryToUI(data);
    } catch (e) {
        console.error('refreshModems', e);
    }
}

async function refreshUSBMode() {
    const el = document.getElementById('usb-mode-current');
    const sel = document.getElementById('usb-mode-select');
    if (!el && !sel) return;
    try {
        const resp = await fetch(apiURL('/api/usbmode'), {headers: apiHeaders(false)});
        if (!resp.ok) return;
        const data = await resp.json();
        if (el) {
            el.innerText = data.mode
                ? (data.label || ('mode ' + data.mode))
                : (data.error || 'unknown');
        }
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
        logConsole('USB mode: pick a mode first');
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
        logConsole('USB mode: ' + (res.status || res.error || '') + ' → ' +
            JSON.stringify(res.usb_mode || {}));
        setTimeout(() => { refreshUSBMode(); refreshModems(); }, 3000);
    } catch (e) {
        logConsole('USB mode error: ' + e);
    }
}

function parseDataIfaceSelection() {
    const val = document.getElementById('data-iface-select').value || '';
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
            logConsole(`Select modem: ${data.error || resp.status}`);
            return;
        }
        applyInventoryToUI(data);
        logConsole(`Using modem ${data.selected_modem_id || modem_id} AT=${data.selected_at_port || at_port || '-'} data=${data.selected_net || data.selected_mbim || '-'}`);
        const st = await fetch(apiURL('/api/status'), {headers: apiHeaders(false)});
        if (st.ok) updateUI(await st.json());
    } catch (e) {
        logConsole(`Select modem error: ${e}`);
    }
}

async function dataConnect() {
    const apn = document.getElementById('apn-name').value || '';
    const sel = parseDataIfaceSelection();
    if (!sel.iface) {
        logConsole('Data connect: no interface — modem may only expose AT ports');
        return;
    }
    try {
        // Persist selection first
        await applyModemSelection();
        const resp = await fetch(apiURL('/api/data/connect'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({mode: sel.mode, iface: sel.iface, apn: apn})
        });
        const res = await resp.json();
        logConsole(`Data connect (${sel.mode} ${sel.iface}): ${res.status || res.error || ''}\n${res.output || ''}`);
        refreshModems();
    } catch (e) {
        logConsole(`Data connect error: ${e}`);
    }
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
        logConsole(`Data disconnect (${sel.mode} ${sel.iface}): ${res.status || res.error || ''}\n${res.output || ''}`);
        refreshModems();
    } catch (e) {
        logConsole(`Data disconnect error: ${e}`);
    }
}

function updateUI(data) {
    const connDot = document.getElementById('conn-dot');
    const connStatus = document.getElementById('conn-status');

    if (data.modem && data.modem.connected) {
        connDot.className = 'dot connected';
        connStatus.innerText = `Connected (${data.modem.port_path || 'USB'})`;
    } else {
        connDot.className = 'dot disconnected';
        connStatus.innerText = 'Disconnected';
    }

    if (data.modem) {
        document.getElementById('modem-port').innerText = data.modem.port_path || '-';
        document.getElementById('modem-power').innerText = data.modem.power_control || '-';
    }

    if (data.signal) {
        document.getElementById('signal-fill').style.width = `${data.signal.percentage || 0}%`;
        document.getElementById('signal-pct').innerText = `${data.signal.percentage || 0}%`;
        document.getElementById('sig-rssi').innerText = data.signal.rssi ? `${data.signal.rssi} dBm` : '-';
        document.getElementById('sig-rsrp').innerText = data.signal.rsrp ? `${data.signal.rsrp} dBm` : '-';
        document.getElementById('sig-rsrq').innerText = data.signal.rsrq ? `${data.signal.rsrq} dB` : '-';
    }

    if (data.network) {
        document.getElementById('net-operator').innerText = data.network.operator || '-';
        document.getElementById('net-tech').innerText = data.network.tech || '-';
        document.getElementById('net-reg').innerText = data.network.reg_state || '-';
    }

    if (data.sim) {
        document.getElementById('sim-state').innerText = data.sim.state || '-';
        document.getElementById('sim-imsi').innerText = data.sim.imsi || '-';
        document.getElementById('sim-iccid').innerText = data.sim.iccid || '-';
    }

    if (data.rat_mode) {
        document.getElementById('rat-mode').innerText = data.rat_mode;
    }

    if (data.apn && data.apn.apn) {
        const apnInput = document.getElementById('apn-name');
        if (document.activeElement !== apnInput) {
            apnInput.value = data.apn.apn;
        }
        if (data.apn.pdp_type) {
            document.getElementById('apn-type').value = data.apn.pdp_type;
        }
    }

    const errEl = document.getElementById('status-error');
    errEl.innerText = data.error || '';
}

async function setRAT(mode) {
    try {
        const resp = await fetch(apiURL('/api/rat'), {
            method: 'POST',
            headers: apiHeaders(true),
            body: JSON.stringify({mode: mode})
        });
        const res = await resp.json();
        logConsole(`Set RAT mode (${mode}): ${res.status || res.error}`);
    } catch (e) {
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
        logConsole(`Update APN (${name}): ${res.status || res.error}`);
    } catch (e) {
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
        logConsole(`USB reset: ${res.status || res.error}`);
    } catch (e) {
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

    // background
    ctx.fillStyle = '#090d16';
    ctx.fillRect(0, 0, w, h);

    const pad = 12;
    const vals = points.map(p => (typeof p.percentage === 'number' ? p.percentage : 0));
    const minV = 0;
    const maxV = 100;

    // grid
    ctx.strokeStyle = '#1e293b';
    ctx.lineWidth = 1;
    for (let y = 0; y <= 4; y++) {
        const yy = pad + ((h - 2 * pad) * y) / 4;
        ctx.beginPath();
        ctx.moveTo(pad, yy);
        ctx.lineTo(w - pad, yy);
        ctx.stroke();
    }

    // line
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

    // last point label
    const last = vals[vals.length - 1];
    ctx.fillStyle = '#94a3b8';
    ctx.font = '12px system-ui, sans-serif';
    ctx.fillText(`${last}% · ${vals.length} samples`, pad, 14);
}

function logConsole(msg) {
    const consoleDiv = document.getElementById('console-output');
    consoleDiv.innerText += `\n${msg}`;
    consoleDiv.scrollTop = consoleDiv.scrollHeight;
}

window.addEventListener('DOMContentLoaded', initSSE);
window.addEventListener('resize', () => drawSignalChart(historyPoints));
