/* Layout shell: panel navigation (sidebar + hash). */

function showPanel(name) {
    const panels = document.querySelectorAll('.panel');
    panels.forEach(p => {
        const active = p.dataset.panel === name;
        p.classList.toggle('active', active);
        if (active) p.removeAttribute('hidden');
        else p.setAttribute('hidden', '');
    });
    document.querySelectorAll('.nav-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.panel === name);
    });
    try {
        sessionStorage.setItem('fm350_panel', name);
    } catch (e) { /* ignore */ }
    if (location.hash !== '#' + name) {
        try {
            history.replaceState(null, '', '#' + name);
        } catch (e) { /* ignore */ }
    }
    if (typeof drawSignalChart === 'function' && (name === 'overview' || name === 'cellular')) {
        try {
            var pts = (typeof historyPoints !== 'undefined') ? historyPoints : [];
            drawSignalChart(pts);
        } catch (e) { /* ignore */ }
    }
}

function initLayout() {
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.addEventListener('click', () => showPanel(btn.dataset.panel));
    });
    let start = 'overview';
    try {
        const h = (location.hash || '').replace(/^#/, '');
        if (h && document.getElementById('panel-' + h)) start = h;
        else {
            const s = sessionStorage.getItem('fm350_panel');
            if (s && document.getElementById('panel-' + s)) start = s;
        }
    } catch (e) { /* ignore */ }
    showPanel(start);
    window.addEventListener('hashchange', () => {
        const h = (location.hash || '').replace(/^#/, '');
        if (h && document.getElementById('panel-' + h)) showPanel(h);
    });
}

window.showPanel = showPanel;
window.initLayout = initLayout;
