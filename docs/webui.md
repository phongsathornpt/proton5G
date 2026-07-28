# WebUI structure (feature-based)

Vanilla SPA embedded via `go:embed` + stdlib `html/template`. **No Node build.**

## Directory layout

```
internal/template/
  embed.go              # //go:embed all:assets
  render.go             # ParseFS + RenderIndex() → full HTML
  render_test.go
  assets/
    layout/
      base.html         # document shell
      topbar.html
      sidebar.html
      content.html      # includes feature panels
      footer.html
      layout.css        # CSS grid: topbar / sidebar / content / footer
      layout.js         # showPanel, initLayout, hash nav
    shared/
      tokens.css
      components.css    # cards, forms, chips, table, toast…
      api.js / utils.js # reserved hooks
    features/
      overview|cellular|wan|lan|advanced/
        panel.html      # {{define "panel-…"}}
        *.css
    app.js              # feature behavior (SSE, WAN, hotspot, …)
    boot.js             # DOMContentLoaded → initLayout + initSSE
```

## Shell regions

```
┌──────────────────────────────────────────┐
│ topbar   logo · AT/WAN/AP pills · AT     │
├──────────┬───────────────────────────────┤
│ sidebar  │ content (feature panels)      │
│ Overview │                               │
│ Cellular │                               │
│ WAN      │                               │
│ LAN      │                               │
│ Advanced │                               │
├──────────┴───────────────────────────────┤
│ footer   last poll · tagline             │
└──────────────────────────────────────────┘
```

Deep links: `#overview` `#cellular` `#wan` `#lan` `#advanced`.

## Serving

| Path | Handler |
|------|---------|
| `GET /` | `template.RenderIndex()` (composed HTML) |
| `GET /assets/*` | `http.FileServer` on embed `assets/` |

## Notes

- All panels stay in the DOM so SSE can update any element id.
- Prefer preserving existing ids when editing feature panels.
- Further split of `app.js` into `features/*/…js` is optional; structure is ready.
