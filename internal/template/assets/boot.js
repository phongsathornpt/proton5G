function bootApp() { if (typeof initLayout === "function") initLayout(); if (typeof initSSE === "function") initSSE(); }
window.addEventListener("DOMContentLoaded", bootApp);
