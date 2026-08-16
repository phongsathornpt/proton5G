// Reduce background API/host work by running auxiliary refreshers only while
// their panel is visible. SSE remains active globally for live modem telemetry.
(function () {
    function panelActive(name) {
        const panel = document.getElementById('panel-' + name);
        return !!(panel && panel.classList.contains('active') && !panel.hasAttribute('hidden'));
    }

    function gateRefresh(name, fn) {
        if (typeof fn !== 'function') return fn;
        return function () {
            if (!panelActive(name)) return;
            return fn.apply(this, arguments);
        };
    }

    const original = {
        history: typeof window.refreshHistory === 'function' ? window.refreshHistory : null,
        modems: typeof window.refreshModems === 'function' ? window.refreshModems : null,
        usbMode: typeof window.refreshUSBMode === 'function' ? window.refreshUSBMode : null,
        hotspot: typeof window.refreshHotspot === 'function' ? window.refreshHotspot : null,
        showPanel: typeof window.showPanel === 'function' ? window.showPanel : null
    };

    if (original.history) window.refreshHistory = gateRefresh('overview', original.history);
    if (original.modems) window.refreshModems = gateRefresh('cellular', original.modems);
    if (original.usbMode) window.refreshUSBMode = gateRefresh('advanced', original.usbMode);
    if (original.hotspot) window.refreshHotspot = gateRefresh('lan', original.hotspot);

    // Global function declarations in classic scripts are window properties, but
    // assign the identifiers too so initSSE's setInterval calls capture the gates.
    if (window.refreshHistory) refreshHistory = window.refreshHistory;
    if (window.refreshModems) refreshModems = window.refreshModems;
    if (window.refreshUSBMode) refreshUSBMode = window.refreshUSBMode;
    if (window.refreshHotspot) refreshHotspot = window.refreshHotspot;

    if (original.showPanel) {
        window.showPanel = function (name) {
            const result = original.showPanel.apply(this, arguments);
            // Force one fresh snapshot when entering a panel so the user never has
            // to wait for its background interval after polling was gated.
            try {
                if (name === 'overview' && original.history) original.history();
                if (name === 'cellular' && original.modems) original.modems();
                if (name === 'advanced' && original.usbMode) original.usbMode();
                if (name === 'lan' && original.hotspot) original.hotspot();
            } catch (e) {
                console.debug('panel refresh skipped', e);
            }
            return result;
        };
        showPanel = window.showPanel;
    }
})();
