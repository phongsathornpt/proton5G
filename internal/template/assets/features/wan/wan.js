/* WAN-specific safety overrides loaded after app.js. */
(function () {
    const originalFillATAndDataForModem = window.fillATAndDataForModem || fillATAndDataForModem;

    window.fillATAndDataForModem = fillATAndDataForModem = function (modem) {
        originalFillATAndDataForModem(modem);

        // A normal MBIM modem exposes both cdc-wdm control and a host net iface.
        // Do not present the host iface as a second, fake RNDIS endpoint.
        if (!modem || (modem.data_mode !== 'mbim' && modem.data_mode !== 'mixed')) return;

        const select = document.getElementById('data-iface-select');
        if (!select) return;

        const safeOptions = (window.__dataIfaceOptions || []).filter(o => o.mode !== 'rndis');
        window.__dataIfaceOptions = safeOptions;

        Array.from(select.options).forEach(option => {
            if (option.value.startsWith('rndis:')) option.remove();
        });

        const selectedMBIM = modemInventory && modemInventory.selected_mbim;
        if (selectedMBIM) {
            const value = 'mbim:' + selectedMBIM;
            if (Array.from(select.options).some(option => option.value === value)) {
                select.value = value;
            }
        }
        select.disabled = select.options.length === 0;
        const btn = document.getElementById('data-connect-btn');
        if (btn) btn.disabled = select.options.length === 0;
    };

    window.dataDisconnect = dataDisconnect = async function () {
        const sel = parseDataIfaceSelection();
        if (!sel.iface) {
            toast('No data interface selected', 'err');
            return;
        }
        try {
            const resp = await fetch(apiURL('/api/data/disconnect'), {
                method: 'POST',
                headers: apiHeaders(true),
                body: JSON.stringify({mode: sel.mode, iface: sel.iface})
            });
            const res = await resp.json();
            const ok = resp.ok && !res.error;
            dataSessionNote = ok ? 'disconnected' : (res.error || 'disconnect failed');
            setText('data-status', dataSessionNote);
            toast(ok ? 'WAN disconnected' : ('Disconnect: ' + (res.error || 'failed')), ok ? 'ok' : 'err');
            logConsole(`Data disconnect (${sel.mode} ${sel.iface}): ${res.status || res.error || ''}\n${res.output || ''}`);
            refreshModems();
            updateOverview();
        } catch (e) {
            dataSessionNote = 'disconnect error';
            setText('data-status', dataSessionNote);
            toast('Disconnect error: ' + e, 'err');
            logConsole(`Data disconnect error: ${e}`);
        }
    };
})();
