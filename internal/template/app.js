let evtSource = null;
let historyPoints = [];
let modemInventory = null;

function initSSE() {
    evtSource = new EventSource('/api/events');

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
    refreshMBIM();
    setInterval(refreshHistory, 15000);
    setInterval(refreshModems, 10000);
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

function fillATAndMBIMForModem(modem) {
    const atOpts = (modem && modem.at_ports ? modem.at_ports : []).map(p => ({
        value: p.path,
        label: p.label || p.path
    }));
    let atSelected = modemInventory && modemInventory.selected_at_port;
    if (modem && atSelected && !atOpts.some(o => o.value === atSelected)) {
        atSelected = atOpts.length ? atOpts[0].value : '';
    }
    fillSelect(document.getElementById('at-port-select'), atOpts, atSelected);

    const mbimOpts = (modem && modem.mbim_nodes ? modem.mbim_nodes : []).map(p => ({
        value: p.path,
        label: p.label || p.path
    }));
    // Also collect all MBIM from inventory if this modem has none
    if (!mbimOpts.length && modemInventory && modemInventory.modems) {
        modemInventory.modems.forEach(m => {
            (m.mbim_nodes || []).forEach(p => {
                if (!mbimOpts.some(o => o.value === p.path)) {
                    mbimOpts.push({value: p.path, label: p.label || p.path});
                }
            });
        });
    }
    fillSelect(document.getElementById('mbim-select'), mbimOpts, modemInventory && modemInventory.selected_mbim);
    const btn = document.getElementById('mbim-connect-btn');
    if (btn) btn.disabled = mbimOpts.length === 0;
}

function onModemDropdownChange() {
    const id = document.getElementById('modem-select').value;
    fillATAndMBIMForModem(findModemById(id));
}

function applyInventoryToUI(data) {
    modemInventory = data;
    const modemOpts = (data.modems || []).map(m => {
        const atN = (m.at_ports || []).length;
        const mbN = (m.mbim_nodes || []).length;
        let label = m.name || m.id;
        label += ` — ${atN} AT`;
        if (mbN) label += `, ${mbN} MBIM`;
        return {value: m.id, label};
    });
    fillSelect(document.getElementById('modem-select'), modemOpts, data.selected_modem_id);
    const modem = findModemById(document.getElementById('modem-select').value) ||
        (data.modems && data.modems[0]) || null;
    fillATAndMBIMForModem(modem);

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
}

async function refreshModems() {
    try {
        const resp = await fetch('/api/modems');
        const data = await resp.json();
        applyInventoryToUI(data);
        // Keep MBIM panel in sync
        document.getElementById('mbim-cli').innerText = data.mbimcli_available ? 'available' : 'missing';
        const mbimHint = document.getElementById('mbim-hint');
        if (mbimHint) {
            if (!data.mbimcli_available && data.install_hint) {
                mbimHint.innerHTML = `Install helper: <code>${data.install_hint}</code> then restart.`;
            } else if (data.note && data.mbimcli_available) {
                mbimHint.textContent = data.note;
            } else {
                mbimHint.innerHTML = `Choose MBIM device (if any) then Connect. Needs <code>mbimcli</code> + <code>/dev/cdc-wdm*</code>.`;
            }
        }
    } catch (e) {
        console.error('refreshModems', e);
    }
}

async function applyModemSelection() {
    const modem_id = document.getElementById('modem-select').value;
    const at_port = document.getElementById('at-port-select').value;
    const mbim_device = document.getElementById('mbim-select').value;
    try {
        const resp = await fetch('/api/modems/select', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({modem_id, at_port, mbim_device})
        });
        const data = await resp.json();
        if (!resp.ok) {
            logConsole(`Select modem: ${data.error || resp.status}`);
            return;
        }
        applyInventoryToUI(data);
        logConsole(`Using modem ${data.selected_modem_id || modem_id} AT=${data.selected_at_port || at_port || '-'}`);
        // Force a status refresh via status endpoint
        const st = await fetch('/api/status');
        if (st.ok) updateUI(await st.json());
    } catch (e) {
        logConsole(`Select modem error: ${e}`);
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
        const resp = await fetch('/api/rat', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
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
        const resp = await fetch('/api/apn', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
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
        const resp = await fetch('/api/raw', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
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
        const resp = await fetch('/api/reset', {method: 'POST'});
        const res = await resp.json();
        logConsole(`USB reset: ${res.status || res.error}`);
    } catch (e) {
        logConsole(`USB reset error: ${e}`);
    }
}

async function refreshMBIM() {
    try {
        const resp = await fetch('/api/mbim');
        const data = await resp.json();
        document.getElementById('mbim-cli').innerText = data.mbimcli_available ? 'available' : 'missing';
        const devices = data.devices || (data.device ? [data.device] : []);
        const opts = devices.map(d => ({value: d, label: d}));
        fillSelect(document.getElementById('mbim-select'), opts, data.selected || data.device);
        const btn = document.getElementById('mbim-connect-btn');
        if (btn) btn.disabled = opts.length === 0;
        const hint = document.getElementById('mbim-hint');
        if (hint) {
            if (!data.mbimcli_available && data.install_hint) {
                hint.innerHTML = `Install helper: <code>${data.install_hint}</code> then restart the manager.`;
            } else if (data.mbimcli_available && !data.device_present) {
                hint.textContent = data.note || 'mbimcli OK, but no /dev/cdc-wdm* — modem may not be in MBIM mode. Use AT port above for monitoring.';
            } else {
                hint.innerHTML = `Select MBIM device then Connect. Requires <code>mbimcli</code> + <code>/dev/cdc-wdm*</code>.`;
            }
        }
    } catch (e) {
        document.getElementById('mbim-cli').innerText = 'error';
    }
}

async function mbimConnect() {
    const apn = document.getElementById('apn-name').value || '';
    const device = document.getElementById('mbim-select').value || '';
    if (!device) {
        logConsole('MBIM connect: no /dev/cdc-wdm* selected — modem may be AT-only');
        return;
    }
    try {
        const resp = await fetch('/api/mbim/connect', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({apn: apn, device: device})
        });
        const res = await resp.json();
        logConsole(`MBIM connect: ${res.status || res.error || ''}\n${res.output || ''}`);
        refreshMBIM();
    } catch (e) {
        logConsole(`MBIM connect error: ${e}`);
    }
}

async function mbimDisconnect() {
    const device = document.getElementById('mbim-select').value || '';
    try {
        const resp = await fetch('/api/mbim/disconnect', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({device: device})
        });
        const res = await resp.json();
        logConsole(`MBIM disconnect: ${res.status || res.error || ''}\n${res.output || ''}`);
        refreshMBIM();
    } catch (e) {
        logConsole(`MBIM disconnect error: ${e}`);
    }
}

async function refreshHistory() {
    try {
        const resp = await fetch('/api/history');
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
