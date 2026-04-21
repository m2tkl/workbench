package workbench

const sharedWebShellCSS = `
    * { box-sizing: border-box; }
    html { background: var(--bg); }
    body {
      margin: 0;
      min-height: 100dvh;
      display: flex;
      flex-direction: column;
      font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: radial-gradient(circle at top, #fbfbfc 0%, var(--bg) 42%);
      color: var(--ink);
      -webkit-font-smoothing: antialiased;
    }
    a { color: inherit; }
    .shell-header,
    main {
      width: min(100%, var(--content-width));
      margin: 0 auto;
    }
    .shell-header {
      padding: 18px 20px 10px;
    }
    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: center;
      flex-wrap: wrap;
    }
    .title-row {
      margin-top: 14px;
    }
    .shell-title {
      margin: 0;
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: baseline;
      font-size: 1.5rem;
      line-height: 1.2;
      font-weight: 600;
      letter-spacing: -0.02em;
    }
    .shell-title .title-link,
    .shell-title .title-current,
    .title-link,
    .title-current {
      display: inline;
      padding: 0;
      border: 0;
      background: transparent;
      font-size: inherit;
      line-height: inherit;
      font-weight: inherit;
      color: inherit;
      text-decoration: none;
      box-shadow: none;
    }
    .shell-title .title-link,
    .title-link { color: var(--muted); }
    .shell-title .crumb-sep,
    .crumb-sep {
      color: var(--muted);
      font-size: 0.88rem;
      font-weight: 400;
    }
    .topbar nav, .breadcrumbs {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    .topbar a, .breadcrumbs a, .breadcrumbs span, .tabs a {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 8px 12px;
      border: 1px solid transparent;
      border-radius: 999px;
      color: var(--muted);
      text-decoration: none;
      font-size: 0.86rem;
      background: transparent;
      transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
    }
    .topbar a:hover, .breadcrumbs a:hover, .tabs a:hover {
      background: rgba(255, 255, 255, 0.72);
      border-color: var(--line);
      color: var(--ink);
    }
    .topbar a.active, .breadcrumbs span, .tabs a.active {
      background: rgba(255, 255, 255, 0.92);
      border-color: var(--line);
      color: var(--ink);
      font-weight: 600;
      box-shadow: 0 1px 1px rgba(15, 23, 42, 0.03);
    }
    a.toolbar-button,
    button.toolbar-button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      width: auto;
      min-width: 0;
      border: 1px solid var(--accent);
      border-radius: 10px;
      padding: 8px 12px;
      font: inherit;
      font-size: 0.86rem;
      font-weight: 500;
      text-decoration: none;
      background: var(--accent);
      color: #fff;
      cursor: pointer;
      box-shadow: 0 10px 24px rgba(15, 23, 42, 0.14);
      transition: background 120ms ease, border-color 120ms ease, box-shadow 120ms ease;
    }
    a.toolbar-button:hover,
    button.toolbar-button:hover {
      background: #0f172a;
      border-color: #0f172a;
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12);
    }
    .capture-modal {
      border: 1px solid var(--line);
      border-radius: 20px;
      padding: 0;
      max-width: min(560px, calc(100vw - 24px));
      width: 100%;
      background: rgba(255, 255, 255, 0.98);
      box-shadow: 0 18px 40px rgba(15, 23, 42, 0.12);
    }
    dialog.capture-modal::backdrop {
      background: rgba(15, 23, 42, 0.28);
      backdrop-filter: blur(4px);
    }
    .capture-card {
      padding: 18px;
      display: grid;
      gap: 14px;
    }
    .capture-head,
    .capture-actions {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    @media (max-width: 920px) {
      .shell-header { padding: 14px 14px 10px; }
      main { padding: 0 14px 14px; }
    }
`

const sharedWebShellHeaderHTML = `
  <header class="shell-header">
    <div class="topbar">
      <nav>
        {{range .HeaderNav}}
        <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>
        {{end}}
      </nav>
      <button id="open-capture" class="toolbar-button" type="button" title="Capture to Inbox (Shift+A)">+ Capture</button>
    </div>
    {{if .TitleNav}}<div class="title-row"><h1 class="shell-title" aria-label="Title navigation">
      {{range $index, $crumb := .TitleNav}}
      {{if $index}}<span class="crumb-sep">/</span>{{end}}
      {{if $crumb.Active}}<span class="title-current">{{$crumb.Label}}</span>{{else}}<a class="title-link" href="{{$crumb.Href}}">{{$crumb.Label}}</a>{{end}}
      {{end}}
    </h1></div>{{end}}
  </header>
`

const sharedCaptureModalHTML = `
    <dialog id="capture-modal" class="capture-modal">
      <div class="capture-card">
        <div class="capture-head">
          <strong>Capture to Inbox</strong>
          <button id="close-capture" type="button">Close</button>
        </div>
        <form method="post" action="{{.CaptureAction}}" class="stack">
          <input type="hidden" name="return_to" value="{{.CaptureReturn}}">
          <input id="capture-title" type="text" name="title" placeholder="Capture a work item" required>
          <div class="capture-actions">
            <button type="submit">+ Add to Inbox</button>
          </div>
        </form>
      </div>
    </dialog>
`

const sharedCaptureModalSetupJS = `
      const captureDialog = document.getElementById("capture-modal");
      const openCaptureButton = document.getElementById("open-capture");
      const closeCaptureButton = document.getElementById("close-capture");
      const captureTitleInput = document.getElementById("capture-title");
      const openCapture = () => {
        if (!captureDialog || typeof captureDialog.showModal !== "function") {
          return;
        }
        captureDialog.showModal();
        window.setTimeout(() => {
          if (captureTitleInput) {
            captureTitleInput.focus();
          }
        }, 0);
      };
      const closeCapture = () => {
        if (captureDialog && captureDialog.open) {
          captureDialog.close();
        }
      };
      if (openCaptureButton) {
        openCaptureButton.addEventListener("click", openCapture);
      }
      if (closeCaptureButton) {
        closeCaptureButton.addEventListener("click", closeCapture);
      }
      document.addEventListener("keydown", (event) => {
        const tag = event.target && event.target.tagName ? String(event.target.tagName).toLowerCase() : "";
        const editable = tag === "input" || tag === "textarea" || tag === "select" || event.target && event.target.isContentEditable;
        if (!editable && !event.metaKey && !event.ctrlKey && !event.altKey && event.shiftKey && String(event.key).toLowerCase() === "a") {
          event.preventDefault();
          openCapture();
          return;
        }
        if (event.key === "Escape" && captureDialog && captureDialog.open) {
          closeCapture();
        }
      });
`

const sharedCaptureModalScript = `
  <script>
    (() => {
` + sharedCaptureModalSetupJS + `
    })();
  </script>
`

const sharedEventPageCSS = `
    .panel {
      padding: 18px;
      display: grid;
      gap: 14px;
      align-content: start;
      background: rgba(255,255,255,0.92);
      border: 1px solid var(--line);
      border-radius: 18px;
      box-shadow: var(--shadow);
    }
    .stack { display: grid; gap: 12px; }
    label { font-size: 0.92rem; font-weight: 600; }
    input[type="text"], select, textarea {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 10px 12px;
      background: var(--surface);
      font: inherit;
      color: var(--ink);
    }
    .message { padding: 12px 14px; border-radius: 14px; border: 1px solid var(--line); background: var(--surface-soft); }
    .message.error { color: var(--error); background: var(--error-bg); border-color: #f3d0cc; }
    .meta, .count, .empty { color: var(--muted); font-size: 0.9rem; }
`

const workbenchHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Workbench</title>
  <style>
    :root {
      --bg: #f5f5f7;
      --surface: #ffffff;
      --surface-soft: #fafafa;
      --surface-muted: #f8fafc;
      --ink: #111827;
      --muted: #6b7280;
      --line: #e5e7eb;
      --line-strong: #d1d5db;
      --accent: #111827;
      --accent-soft: #eef2f7;
      --error: #b42318;
      --error-bg: #fef3f2;
      --ok-bg: #f8fafc;
      --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 12px 30px rgba(15, 23, 42, 0.05);
      --shadow-popover: 0 18px 40px rgba(15, 23, 42, 0.12);
      --content-width: 1480px;
      --sidebar-expanded-width: 280px;
      --pane-header-height: 56px;
    }
` + sharedWebShellCSS + `
    body { height: 100dvh; overflow: hidden; }
    main {
      flex: 1 1 auto;
      min-height: 0;
      display: flex;
      flex-direction: column;
      padding: 0 20px 20px;
      overflow: hidden;
    }
    h1, h2, h3 {
      margin: 0;
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    .meta, .empty, .count {
      color: var(--muted);
      font-size: 0.92rem;
    }
    .notice {
      padding: 11px 14px;
      border: 1px solid var(--line);
      border-radius: 12px;
      margin: 0 0 14px;
      font-size: 0.92rem;
      background: var(--ok-bg);
      box-shadow: 0 1px 1px rgba(15, 23, 42, 0.02);
    }
    .notice.ok { color: var(--ink); }
    .notice.error {
      color: var(--error);
      background: var(--error-bg);
      border-color: #f3d0cc;
    }
    .layout {
      display: grid;
      grid-template-columns: var(--sidebar-expanded-width) minmax(0, 1fr);
      gap: 16px;
      align-items: stretch;
      flex: 1 1 auto;
      min-height: 0;
    }
    .layout[data-sidebar-collapsed="true"] {
      grid-template-columns: 56px minmax(0, 1fr);
    }
    .panel {
      border: 1px solid rgba(229, 231, 235, 0.9);
      border-radius: 18px;
      background: rgba(255, 255, 255, 0.86);
      box-shadow: var(--shadow);
      backdrop-filter: blur(10px);
    }
    .sidebar {
      position: relative;
      display: flex;
      flex-direction: column;
      min-height: 0;
      height: 100%;
      overflow: hidden;
      padding: 0;
      background: rgba(255, 255, 255, 0.78);
    }
    .sidebar-toolbar {
      display: flex;
      justify-content: flex-start;
      align-items: center;
      gap: 10px;
      min-height: var(--pane-header-height);
      padding: 12px 12px 10px;
      border-bottom: 1px solid rgba(229, 231, 235, 0.8);
      background: rgba(255, 255, 255, 0.72);
      backdrop-filter: blur(8px);
    }
    .sidebar-title,
    .pane-header .section-label {
      font-size: 0.74rem;
      font-weight: 600;
      color: var(--muted);
      letter-spacing: 0.06em;
      text-transform: uppercase;
    }
    .sidebar-content {
      display: flex;
      flex-direction: column;
      gap: 18px;
      min-height: 0;
      padding: 14px;
      overflow: auto;
    }
    .nav-group + .nav-group {
      border-top: 1px solid rgba(229, 231, 235, 0.8);
      padding-top: 16px;
    }
    .layout[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-title,
    .layout[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-content {
      display: none;
    }
    .layout[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-toolbar {
      border-bottom: 0;
    }
    .layout[data-sidebar-collapsed="true"][data-sidebar-hovered="true"] .sidebar {
      width: min(var(--sidebar-expanded-width), calc(100vw - 32px));
      z-index: 3;
      box-shadow: var(--shadow-popover);
    }
    .content,
    .content-panel {
      display: flex;
      flex: 1 1 auto;
      flex-direction: column;
      min-height: 0;
    }
    .content { gap: 16px; }
    .content-panel {
      padding: 0;
      overflow: hidden;
    }
    .pane-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      min-height: var(--pane-header-height);
      padding: 14px 18px 12px;
      border-bottom: 1px solid rgba(229, 231, 235, 0.8);
    }
    .content-panel-body {
      flex: 1 1 auto;
      min-height: 0;
      padding: 10px 18px 14px;
      overflow: auto;
    }
    .content-tabs {
      display: flex;
      align-items: flex-end;
      gap: 22px;
      padding: 0 4px;
      border-bottom: 1px solid rgba(226, 232, 240, 0.96);
      overflow-x: auto;
      scrollbar-width: none;
    }
    .content-tabs::-webkit-scrollbar {
      display: none;
    }
    .content-tabs a {
      position: relative;
      display: inline-flex;
      align-items: center;
      padding: 0 0 12px;
      border-radius: 0;
      color: #64748b;
      text-decoration: none;
      font-size: 0.9rem;
      font-weight: 500;
      white-space: nowrap;
      background: transparent;
    }
    .content-tabs a::after {
      content: "";
      position: absolute;
      left: 0;
      right: 0;
      bottom: -1px;
      height: 2px;
      border-radius: 999px;
      background: transparent;
    }
    .content-tabs a:hover {
      color: var(--ink);
    }
    .content-tabs a.active {
      color: var(--ink);
    }
    .content-tabs a.active::after {
      background: rgba(17, 24, 39, 0.96);
    }
    .theme-project-shell {
      display: grid;
      gap: 18px;
      width: min(100%, 880px);
      margin: 0 auto;
      padding: 6px 0 10px;
    }
    .theme-tab-toolbar {
      display: flex;
      justify-content: flex-start;
      align-items: center;
      gap: 12px;
      padding: 2px 2px 4px;
      flex-wrap: wrap;
    }
    .theme-tab-form {
      display: flex;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;
      width: 100%;
    }
    .theme-tab-form input[type="text"] {
      flex: 1 1 320px;
      min-width: min(100%, 260px);
    }
    .theme-tab-form .toolbar-button,
    .theme-tab-toolbar .toolbar-button {
      width: auto;
      min-width: 0;
      white-space: nowrap;
    }
    .sidebar-toggle,
    .link-button,
    button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 8px 12px;
      font: inherit;
      font-size: 0.86rem;
      font-weight: 500;
      text-decoration: none;
      background: rgba(255, 255, 255, 0.94);
      color: var(--ink);
      cursor: pointer;
      transition: background 120ms ease, border-color 120ms ease, box-shadow 120ms ease;
    }
    .link-button:hover,
    .sidebar-toggle:hover,
    button:hover {
      border-color: var(--line-strong);
      background: #ffffff;
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
    }
    .sidebar-toggle {
      width: 32px;
      min-width: 32px;
      height: 32px;
      padding: 0;
      flex: 0 0 32px;
      font-size: 14px;
      line-height: 1;
      border-radius: 9px;
      box-shadow: none;
    }
    form.inline {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    input[type="text"], select {
      border-radius: 10px;
      border: 1px solid var(--line);
      padding: 9px 12px;
      font: inherit;
      background: rgba(255, 255, 255, 0.98);
      color: var(--ink);
      transition: border-color 120ms ease, box-shadow 120ms ease;
    }
    input[type="text"]:focus, select:focus {
      outline: none;
      border-color: #c7d2fe;
      box-shadow: 0 0 0 4px rgba(99, 102, 241, 0.08);
    }
    input[type="text"] { width: 100%; }
    .nav-group-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      margin-bottom: 10px;
    }
    .nav-group h2 {
      margin: 0;
      padding-left: 10px;
      color: var(--muted);
      font-size: 0.8rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .theme-create {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: 0;
      border-radius: 999px;
      padding: 0;
      background: transparent;
      color: var(--muted);
      cursor: pointer;
      transition: background 120ms ease, color 120ms ease;
    }
    .theme-create:hover {
      background: rgba(241, 245, 249, 0.98);
      color: var(--ink);
    }
    .theme-create .icon {
      font-size: 1rem;
      line-height: 1;
    }
    .theme-create-card {
      padding: 18px;
      display: grid;
      gap: 14px;
    }
    .theme-create-form {
      display: grid;
      gap: 12px;
    }
    .theme-create-form input[type="text"] {
      min-height: 36px;
      padding: 8px 12px;
      border-radius: 12px;
      border: 1px solid rgba(226, 232, 240, 0.96);
      background: rgba(255, 255, 255, 0.98);
      font-size: 0.9rem;
      box-shadow: none;
    }
    .theme-create-form-actions {
      display: flex;
      justify-content: flex-end;
    }
    .theme-create-form button {
      width: auto;
      min-width: 0;
      min-height: 32px;
      padding: 0 12px;
      border-radius: 999px;
      font-size: 0.84rem;
      font-weight: 600;
    }
    .nav-list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 4px;
    }
    .nav-list a {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      padding: 9px 10px;
      border-radius: 12px;
      text-decoration: none;
      color: var(--muted);
      transition: background 120ms ease, color 120ms ease;
    }
    .nav-list a:hover {
      background: var(--surface-muted);
      color: var(--ink);
    }
    .nav-list a.active {
      background: var(--accent-soft);
      color: var(--ink);
      font-weight: 600;
    }
    .nav-list .count {
      font-size: 0.84rem;
    }
    .item-title {
      font-weight: 600;
      text-decoration: none;
      letter-spacing: -0.01em;
    }
    .item-title:hover {
      text-decoration: underline;
      text-decoration-color: rgba(17, 24, 39, 0.28);
    }
    .workbench-list {
      display: grid;
      gap: 0;
      padding: 2px 0 0;
    }
    .workbench-row {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 16px;
      align-items: center;
      padding: 14px 6px;
      border-top: 1px solid rgba(229, 231, 235, 0.8);
      background: transparent;
      transition: background 120ms ease;
    }
    .workbench-row:first-child {
      border-top: 0;
    }
    .workbench-row:hover {
      background: rgba(255, 255, 255, 0.56);
    }
    .source-list {
      display: grid;
      gap: 0;
      padding: 2px 0 0;
    }
    .source-row {
      display: grid;
      gap: 6px;
      padding: 14px 6px;
      border-top: 1px solid rgba(229, 231, 235, 0.8);
    }
    .source-row:first-child {
      border-top: 0;
    }
    .source-title {
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    .source-ref {
      color: var(--muted);
      font-size: 0.84rem;
      line-height: 1.5;
      word-break: break-all;
    }
    .workbench-row-main {
      min-width: 0;
      display: grid;
      gap: 8px;
    }
    .item-stack {
      display: grid;
      gap: 6px;
      min-width: 0;
    }
    .item-stack .meta {
      font-size: 0.88rem;
      line-height: 1.55;
    }
    .row-meta-line {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      align-items: center;
    }
    .theme-inline,
    .row-summary,
    .stage-inline {
      color: var(--muted);
      font-size: 0.85rem;
      line-height: 1.5;
      white-space: nowrap;
    }
    .theme-inline {
      color: var(--ink);
    }
    .stage-inline {
      display: inline-flex;
      align-items: center;
    }
    .row-summary {
      white-space: normal;
    }
    .workbench-row-side {
      display: flex;
      align-items: center;
      justify-content: end;
      align-self: start;
      width: 30px;
      min-width: 0;
      padding-top: 2px;
    }
    .row-actions {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      width: 30px;
    }
    .menu-form-grid {
      display: grid;
      gap: 10px;
    }
    .menu-form-grid form {
      display: grid;
      gap: 6px;
      margin: 0;
    }
    .menu-divider {
      height: 1px;
      margin: 2px 0 0;
      background: rgba(229, 231, 235, 0.9);
    }
    .menu-form-grid select {
      width: 100%;
      min-width: 0;
      min-height: 34px;
      padding: 8px 10px;
      border-color: rgba(226, 232, 240, 0.96);
      background: rgba(255, 255, 255, 0.98);
      box-shadow: none;
      color: var(--ink);
      font-size: 0.84rem;
    }
    .menu-form-grid button {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      justify-self: stretch;
      width: 100%;
      min-height: 32px;
      padding: 7px 10px;
      border: 0;
      border-radius: 8px;
      background: transparent;
      border-color: transparent;
      color: var(--ink);
      box-shadow: none;
      white-space: nowrap;
      text-align: left;
      font-size: 0.88rem;
      font-weight: 500;
    }
    .menu-action-icon {
      width: 15px;
      height: 15px;
      flex: 0 0 15px;
      stroke: currentColor;
      fill: none;
      stroke-width: 1.6;
      stroke-linecap: round;
      stroke-linejoin: round;
      opacity: 0.82;
    }
    .menu-action-label {
      flex: 1 1 auto;
      min-width: 0;
    }
    .menu-form-grid button:hover {
      background: rgba(241, 245, 249, 0.92);
    }
    .row-menu {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      grid-column: 2;
    }
    .row-menu summary {
      list-style: none;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 30px;
      height: 30px;
      border: 1px solid transparent;
      border-radius: 999px;
      background: transparent;
      color: var(--muted);
      cursor: pointer;
      line-height: 0;
    }
    .row-menu summary::-webkit-details-marker { display: none; }
    .row-menu-icon {
      display: block;
      width: 14px;
      height: 14px;
      fill: currentColor;
    }
    .row-menu[open] summary {
      background: rgba(255, 255, 255, 0.86);
      border-color: var(--line);
      color: var(--ink);
    }
    .row-menu-popover {
      position: fixed;
      left: 0;
      top: 0;
      z-index: 40;
      min-width: 232px;
      max-width: min(280px, calc(100vw - 24px));
      max-height: calc(100vh - 24px);
      padding: 10px;
      border: 1px solid rgba(229, 231, 235, 0.96);
      border-radius: 12px;
      background: rgba(255, 255, 255, 0.97);
      box-shadow: var(--shadow-popover);
      display: none;
      gap: 6px;
      overflow: auto;
      backdrop-filter: blur(10px);
    }
    .row-menu-popover.row-menu-popover-mounted {
      display: grid;
    }
    .row-menu-popover .meta-label {
      color: var(--muted);
      font-size: 0.72rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .row-menu-popover select {
      width: 100%;
      min-width: 0;
    }
    .empty {
      padding: 18px 2px 6px;
      line-height: 1.5;
    }
    .capture-head button,
    .capture-actions button {
      width: auto;
    }
    .capture-actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    @media (max-width: 920px) {
      .shell-header { padding: 14px 14px 10px; }
      main { padding: 0 14px 14px; }
      .layout {
        grid-template-columns: minmax(220px, var(--sidebar-expanded-width)) minmax(0, 1fr);
      }
      .workbench-row {
        gap: 12px;
        padding: 10px 4px;
      }
      .source-row {
        padding: 10px 4px;
      }
      .item-stack {
        gap: 4px;
      }
      .row-meta-line {
        gap: 8px;
      }
      .workbench-row-side {
        align-self: center;
        padding-top: 0;
      }
      .row-actions {
        width: 30px;
      }
    }
    @media (max-width: 720px) {
      .workbench-row {
        grid-template-columns: 1fr;
        align-items: start;
      }
      .content-tabs {
        gap: 18px;
      }
      .theme-tab-form {
        align-items: stretch;
      }
      .workbench-row-side {
        align-self: start;
      }
    }
  </style>
</head>
<body>
` + sharedWebShellHeaderHTML + `
  <main>
    {{if .Status}}<div class="notice ok">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}

    <div class="layout" data-sidebar-collapsed="false" data-sidebar-hovered="false">
      <aside class="panel sidebar">
        <div class="sidebar-toolbar">
          <button id="toggle-sidebar" class="sidebar-toggle" type="button" aria-expanded="true" aria-controls="workbench-sidebar-content" title="Toggle sidebar">&#9664;</button>
          <div class="sidebar-title">Explorer</div>
        </div>
        <div id="workbench-sidebar-content" class="sidebar-content">
          {{range .NavGroups}}
          <section class="nav-group">
            <div class="nav-group-head">
              <h2>{{.Label}}</h2>
              {{if .ShowCreateControl}}
              <button type="button" class="theme-create" aria-label="Create theme"><span class="icon">+</span></button>
              {{end}}
            </div>
            <ul class="nav-list">
              {{range .Entries}}
              <li><a href="{{.Href}}"{{if .Active}} class="active"{{end}}><span>{{.Title}}</span><span class="count">{{.Count}}</span></a></li>
              {{end}}
            </ul>
            {{if .ShowCreateControl}}
            <dialog id="theme-create-modal" class="capture-modal" data-open-on-load="{{if .CreateOpen}}true{{else}}false{{end}}">
              <div class="theme-create-card">
                <div class="capture-head">
                  <strong>Create Theme</strong>
                  <button id="close-theme-create" type="button">Close</button>
                </div>
                <form method="post" action="{{.CreateAction}}" class="theme-create-form">
                  <input type="hidden" name="nav" value="{{.CreateNav}}">
                  <input type="hidden" name="tab" value="{{.CreateTab}}">
                  <input type="hidden" name="q" value="{{.CreateQuery}}">
                  <input id="theme-create-title" type="text" name="title" placeholder="{{.CreatePlaceholder}}" aria-label="Theme title" required>
                  <div class="theme-create-form-actions">
                    <button type="submit">{{.CreateButtonLabel}}</button>
                  </div>
                </form>
              </div>
            </dialog>
            {{end}}
          </section>
          {{end}}
        </div>
      </aside>

      <section class="content">
        <section class="panel content-panel">
          <div class="pane-header">
            <div class="section-label">{{.CurrentTitle}}</div>
            <div class="count">{{.CurrentCountLabel}}</div>
          </div>
          <div class="content-panel-body">
          {{if .ThemeTabs}}<div class="theme-project-shell">
          <nav class="content-tabs" aria-label="Theme view">
            {{range .ThemeTabs}}
            <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>
            {{end}}
          </nav>
          {{end}}
          {{if .ShowThemeComposer}}
          <div class="theme-tab-toolbar">
            <form method="post" action="{{.ThemeComposerAction}}" class="theme-tab-form">
              <input type="hidden" name="nav" value="{{.Nav}}">
              <input type="hidden" name="q" value="{{.Query}}">
              <input type="hidden" name="theme_id" value="{{.ThemeComposerThemeID}}">
              <input type="hidden" name="return_to" value="{{.CaptureReturn}}">
              <input type="text" name="title" placeholder="{{.ThemeComposerPlaceholder}}" aria-label="Add work item" required>
              <button class="toolbar-button" type="submit">Add Work Item</button>
            </form>
          </div>
          {{if .Items}}
          <div class="workbench-list">
            {{range .Items}}
            <article class="workbench-row">
              <div class="workbench-row-main">
                <div class="item-stack">
                  <a class="item-title" href="{{.WorkspaceHref}}">{{.Title}}</a>
                  <div class="row-meta-line">
                    {{if .ThemeLabel}}<span class="theme-inline">{{.ThemeLabel}}</span>{{end}}
                    {{if .StageLabel}}<span class="stage-inline">{{.StageLabel}}</span>{{end}}
                    {{if .Summary}}<span class="row-summary">{{.Summary}}</span>{{end}}
                  </div>
                </div>
              </div>
              <div class="workbench-row-side">
                <div class="row-actions">
                  <details class="row-menu">
                    <summary aria-label="More actions for {{.Title}}"><svg class="row-menu-icon" aria-hidden="true" viewBox="0 0 16 16"><circle cx="3" cy="8" r="1.25"></circle><circle cx="8" cy="8" r="1.25"></circle><circle cx="13" cy="8" r="1.25"></circle></svg></summary>
                    <div class="row-menu-popover" role="menu">
                      <div class="menu-section">
                        <form method="post" action="{{.ThemeAction}}" class="menu-form-grid">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <label>
                            <span>Theme</span>
                            <select name="theme">
                              {{range .ThemeOptions}}
                              <option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>
                              {{end}}
                            </select>
                          </label>
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M2.75 8h10.5"></path><path d="M8 2.75v10.5"></path></svg><span class="menu-action-label">Set theme</span></button>
                        </form>
                      </div>
                      <div class="menu-divider"></div>
                      <div class="menu-section">
                        <form method="post" action="{{.MoveAction}}" class="menu-form-grid">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <label>
                            <span>Stage</span>
                            <select name="stage">
                              {{range .MoveOptions}}
                              <option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>
                              {{end}}
                            </select>
                          </label>
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M3 4.25h10"></path><path d="M3 8h7"></path><path d="M3 11.75h4"></path></svg><span class="menu-action-label">Update stage</span></button>
                        </form>
                      </div>
                      {{if .CanDoneForDay}}
                      <div class="menu-divider"></div>
                      <div class="menu-section">
                        <form method="post" action="{{.DoneForDayAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M8 2.75v10.5"></path><path d="M4.25 6.5L8 2.75 11.75 6.5"></path><path d="M3.5 13.25h9"></path></svg><span class="menu-action-label">Done for today</span></button>
                        </form>
                      </div>
                      {{end}}
                      {{if .CanComplete}}
                      <div class="menu-divider"></div>
                      <div class="menu-section">
                        <form method="post" action="{{.CompleteAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M3.5 8.5 6.5 11.5 12.5 4.5"></path></svg><span class="menu-action-label">Done</span></button>
                        </form>
                      </div>
                      {{end}}
                      {{if and .CanReopen .CanReopenDoneForDay}}
                      <div class="menu-divider"></div>
                      <div class="menu-section">
                        <form method="post" action="{{.ReopenAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M5 5H2.75v2.25"></path><path d="M3 7.25a5 5 0 1 0 1.7-3.75"></path></svg><span class="menu-action-label">Resume today</span></button>
                        </form>
                      </div>
                      {{end}}
                      {{if and .CanReopen .CanReopenComplete}}
                      <div class="menu-divider"></div>
                      <div class="menu-section">
                        <form method="post" action="{{.ReopenAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M5 5H2.75v2.25"></path><path d="M3 7.25a5 5 0 1 0 1.7-3.75"></path></svg><span class="menu-action-label">Reopen item</span></button>
                        </form>
                      </div>
                      {{end}}
                    </div>
                  </details>
                </div>
              </div>
            </article>
            {{end}}
          </div>
          {{else}}
          <div class="empty">{{.EmptyState}}</div>
          {{end}}
          {{else if .ShowThemeSources}}
          <div class="theme-tab-toolbar">
            <a class="toolbar-button" href="{{.ThemeAddSourcesHref}}">Add Sources</a>
          </div>
          {{if .ThemeSources}}
          <div class="source-list">
            {{range .ThemeSources}}
            <article class="source-row">
              <div class="source-title">{{.Title}}</div>
              <div class="source-ref">{{.Ref}}</div>
            </article>
            {{end}}
           </div>
           {{else}}
           <div class="empty">{{.EmptyState}}</div>
           {{end}}
          {{else if .ShowThemeEvents}}
          <div class="theme-tab-toolbar">
            <a class="toolbar-button" href="{{.ThemeAddEventsHref}}">Add Event</a>
          </div>
          {{if .ThemeEvents}}
          <div class="source-list">
            {{range .ThemeEvents}}
            <article class="source-row">
              <div>
                <a class="item-title" href="{{.Href}}">{{.Title}}</a>
                {{if .Updated}}<div class="meta">Updated {{.Updated}}</div>{{end}}
              </div>
            </article>
            {{end}}
          </div>
          {{else}}
          <div class="empty">{{.EmptyState}}</div>
          {{end}}
          {{else if .Items}}
          <div class="workbench-list">
            {{range .Items}}
            <article class="workbench-row">
              <div class="workbench-row-main">
                <div class="item-stack">
                  <a class="item-title" href="{{.WorkspaceHref}}">{{.Title}}</a>
                  <div class="row-meta-line">
                    {{if .ThemeLabel}}<span class="theme-inline">{{.ThemeLabel}}</span>{{end}}
                    {{if .StageLabel}}<span class="stage-inline">{{.StageLabel}}</span>{{end}}
                    {{if .Summary}}<span class="row-summary">{{.Summary}}</span>{{end}}
                  </div>
                </div>
              </div>
              <div class="workbench-row-side">
                <div class="row-actions">
                  <details class="row-menu">
                    <summary aria-label="More actions for {{.Title}}"><svg class="row-menu-icon" aria-hidden="true" viewBox="0 0 16 16"><circle cx="3" cy="8" r="1.25"></circle><circle cx="8" cy="8" r="1.25"></circle><circle cx="13" cy="8" r="1.25"></circle></svg></summary>
                    <div class="row-menu-popover">
                      <div class="menu-form-grid">
                        {{if .CanMove}}
                        <form method="post" action="{{.MoveAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <div class="meta-label">Stage</div>
                          <select name="to" aria-label="Set stage for {{.Title}}">
                            {{range .MoveOptions}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
                          </select>
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M3 4.5h10"></path><path d="M3 8h7"></path><path d="M3 11.5h4"></path></svg><span class="menu-action-label">Update stage</span></button>
                        </form>
                        {{end}}
                        {{if .CanSetTheme}}
                        <form method="post" action="{{.ThemeAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <div class="meta-label">Theme</div>
                          <select name="theme_id" aria-label="Set theme for {{.Title}}">
                            {{range .ThemeOptions}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
                          </select>
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M8 2.5l1.5 3 3.3.5-2.4 2.3.6 3.2L8 9.9l-3 1.6.6-3.2L3.2 6l3.3-.5z"></path></svg><span class="menu-action-label">Set theme</span></button>
                        </form>
                        {{end}}
                        {{if or .CanDoneForDay .CanComplete .CanReopen}}
                        <div class="menu-divider" aria-hidden="true"></div>
                        {{end}}
                        {{if .CanDoneForDay}}
                        <form method="post" action="{{.DoneForDayAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M5.25 2.75v2"></path><path d="M10.75 2.75v2"></path><path d="M3 6.25h10"></path><rect x="3" y="4.25" width="10" height="8.75" rx="2"></rect><path d="M6 9l1.4 1.4L10.25 7.5"></path></svg><span class="menu-action-label">Done for today</span></button>
                        </form>
                        {{end}}
                        {{if .CanComplete}}
                        <form method="post" action="{{.CompleteAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.25"></circle><path d="M5.8 8.1l1.45 1.45L10.4 6.4"></path></svg><span class="menu-action-label">Done</span></button>
                        </form>
                        {{end}}
                        {{if and .CanReopen .CanReopenDoneForDay}}
                        <form method="post" action="{{.ReopenAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M5 5H2.75v2.25"></path><path d="M3 7.25a5 5 0 1 0 1.7-3.75"></path></svg><span class="menu-action-label">Restore for today</span></button>
                        </form>
                        {{end}}
                        {{if and .CanReopen .CanReopenComplete}}
                        <form method="post" action="{{.ReopenAction}}">
                          <input type="hidden" name="q" value="{{$.Query}}">
                          <input type="hidden" name="nav" value="{{$.Nav}}">
                          <button type="submit"><svg class="menu-action-icon" aria-hidden="true" viewBox="0 0 16 16"><path d="M5 5H2.75v2.25"></path><path d="M3 7.25a5 5 0 1 0 1.7-3.75"></path></svg><span class="menu-action-label">Reopen item</span></button>
                        </form>
                        {{end}}
                      </div>
                    </div>
                  </details>
                </div>
              </div>
            </article>
            {{end}}
          </div>
          {{else}}
          <div class="empty">{{.EmptyState}}</div>
          {{end}}
          {{if .ThemeTabs}}</div>{{end}}
          </div>
        </section>
      </section>
    </div>
` + sharedCaptureModalHTML + `
  </main>
  <script>
    (() => {
      const layout = document.querySelector(".layout");
      const sidebar = document.querySelector(".sidebar");
      const toggleSidebarButton = document.getElementById("toggle-sidebar");
      const sidebarStateKey = "workbench.sidebar.collapsed";
` + sharedCaptureModalSetupJS + `
      const themeDialog = document.getElementById("theme-create-modal");
      const openThemeButton = document.querySelector(".theme-create");
      const closeThemeButton = document.getElementById("close-theme-create");
      const themeTitleInput = document.getElementById("theme-create-title");
      const rowMenus = Array.from(document.querySelectorAll(".row-menu"));
      let activeRowMenu = null;
      let activeRowMenuPopover = null;
      let activeRowMenuPlaceholder = null;
      const sidebarCollapsed = () => layout && layout.dataset.sidebarCollapsed === "true";
      const syncSidebarState = () => {
        if (!layout || !toggleSidebarButton) {
          return;
        }
        const collapsed = sidebarCollapsed();
        const hovered = layout.dataset.sidebarHovered === "true";
        const expanded = !collapsed || hovered;
        toggleSidebarButton.setAttribute("aria-expanded", expanded ? "true" : "false");
        toggleSidebarButton.innerHTML = expanded ? "&#9664;" : "&#9654;";
      };
      const setSidebarCollapsed = (collapsed) => {
        if (!layout) {
          return;
        }
        layout.dataset.sidebarCollapsed = collapsed ? "true" : "false";
        window.localStorage.setItem(sidebarStateKey, collapsed ? "true" : "false");
        if (!collapsed) {
          layout.dataset.sidebarHovered = "false";
        }
        syncSidebarState();
      };
      const setSidebarHovered = (hovered) => {
        if (!layout) {
          return;
        }
        layout.dataset.sidebarHovered = sidebarCollapsed() && hovered ? "true" : "false";
        syncSidebarState();
      };
      const openThemeCreate = () => {
        if (!themeDialog || themeDialog.open || typeof themeDialog.showModal !== "function") {
          return;
        }
        themeDialog.showModal();
        window.setTimeout(() => {
          if (themeTitleInput) {
            themeTitleInput.focus();
          }
        }, 0);
      };
      const closeThemeCreate = () => {
        if (themeDialog && themeDialog.open) {
          themeDialog.close();
        }
      };
      const unmountRowMenu = () => {
        if (!activeRowMenu || !activeRowMenuPopover || !activeRowMenuPlaceholder) {
          return;
        }
        activeRowMenuPlaceholder.replaceWith(activeRowMenuPopover);
        activeRowMenuPopover.classList.remove("row-menu-popover-mounted");
        activeRowMenuPopover.style.left = "0px";
        activeRowMenuPopover.style.top = "0px";
        activeRowMenuPopover.style.removeProperty("max-height");
        activeRowMenu = null;
        activeRowMenuPopover = null;
        activeRowMenuPlaceholder = null;
      };
      const closeActiveRowMenu = () => {
        if (!activeRowMenu) {
          return;
        }
        const details = activeRowMenu;
        details.open = false;
        unmountRowMenu();
      };
      const positionActiveRowMenu = () => {
        if (!activeRowMenu || !activeRowMenuPopover) {
          return;
        }
        const summary = activeRowMenu.querySelector("summary");
        if (!summary) {
          return;
        }
        const rect = summary.getBoundingClientRect();
        const gap = 8;
        const margin = 12;
        activeRowMenuPopover.style.left = "0px";
        activeRowMenuPopover.style.top = "0px";
        activeRowMenuPopover.style.maxHeight = "calc(100vh - " + (margin*2) + "px)";
        const popoverRect = activeRowMenuPopover.getBoundingClientRect();
        const width = popoverRect.width;
        const height = popoverRect.height;
        const left = Math.max(margin, rect.left - gap - width);
        const top = Math.max(margin, Math.min(rect.top, window.innerHeight - height - margin));
        activeRowMenuPopover.style.left = Math.round(left) + "px";
        activeRowMenuPopover.style.top = Math.round(top) + "px";
      };
      const mountRowMenu = (details) => {
        const popover = details.querySelector(".row-menu-popover");
        if (!popover) {
          return;
        }
        if (activeRowMenu && activeRowMenu !== details) {
          closeActiveRowMenu();
        }
        if (activeRowMenu === details && activeRowMenuPopover === popover) {
          positionActiveRowMenu();
          return;
        }
        const placeholder = document.createElement("span");
        placeholder.hidden = true;
        popover.before(placeholder);
        document.body.appendChild(popover);
        popover.classList.add("row-menu-popover-mounted");
        activeRowMenu = details;
        activeRowMenuPopover = popover;
        activeRowMenuPlaceholder = placeholder;
        positionActiveRowMenu();
      };
      if (openThemeButton) {
        openThemeButton.addEventListener("click", openThemeCreate);
      }
      if (closeThemeButton) {
        closeThemeButton.addEventListener("click", closeThemeCreate);
      }
      if (toggleSidebarButton) {
        toggleSidebarButton.addEventListener("click", () => {
          setSidebarCollapsed(!sidebarCollapsed());
        });
      }
      if (sidebar) {
        sidebar.addEventListener("mouseenter", () => setSidebarHovered(true));
        sidebar.addEventListener("mouseleave", () => setSidebarHovered(false));
      }
      rowMenus.forEach((details) => {
        details.addEventListener("toggle", () => {
          if (details.open) {
            mountRowMenu(details);
            return;
          }
          if (activeRowMenu === details) {
            unmountRowMenu();
          }
        });
      });
      const persistedSidebarState = window.localStorage.getItem(sidebarStateKey);
      if (persistedSidebarState === "true" || persistedSidebarState === "false") {
        layout.dataset.sidebarCollapsed = persistedSidebarState;
      }
      syncSidebarState();
      if (themeDialog && themeDialog.dataset.openOnLoad === "true") {
        openThemeCreate();
      }
      document.addEventListener("click", (event) => {
        if (!activeRowMenu || !activeRowMenuPopover) {
          return;
        }
        const target = event.target;
        if (target instanceof Node && (activeRowMenu.contains(target) || activeRowMenuPopover.contains(target))) {
          return;
        }
        closeActiveRowMenu();
      });
      window.addEventListener("resize", positionActiveRowMenu);
      window.addEventListener("scroll", positionActiveRowMenu, true);
      document.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && activeRowMenu) {
          closeActiveRowMenu();
          return;
        }
        if (event.key === "Escape" && themeDialog && themeDialog.open) {
          closeThemeCreate();
        }
      });
    })();
  </script>
</body>
</html>`

const sourceWorkbenchHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Workbench Sources</title>
  <style>
    :root {
      --bg: #f5f5f7;
      --surface: #ffffff;
      --surface-soft: #fafafa;
      --surface-muted: #f8fafc;
      --ink: #111827;
      --muted: #6b7280;
      --line: #e5e7eb;
      --line-strong: #d1d5db;
      --accent: #111827;
      --accent-soft: #eef2f7;
      --error: #b42318;
      --error-bg: #fef3f2;
      --ok-bg: #f8fafc;
      --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 12px 30px rgba(15, 23, 42, 0.05);
      --shadow-popover: 0 18px 40px rgba(15, 23, 42, 0.12);
      --content-width: 1480px;
    }
` + sharedWebShellCSS + `
    main {
      flex: 1 1 auto;
      min-height: 0;
      padding: 0 20px 20px;
    }
    h1 {
      margin: 0;
      font-size: 1.5rem;
      font-weight: 600;
      letter-spacing: -0.02em;
    }
    h2 {
      margin: 0;
      font-size: 1.1rem;
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    p.lead, .meta, .empty {
      color: var(--muted);
      font-size: 0.93rem;
      line-height: 1.6;
    }
    p.lead {
      margin: 0 0 14px;
    }
    .section {
      padding: 0;
      margin-bottom: 18px;
    }
    .panel {
      border: 1px solid rgba(229, 231, 235, 0.9);
      border-radius: 20px;
      padding: 20px;
      background: rgba(255, 255, 255, 0.88);
      box-shadow: var(--shadow);
      backdrop-filter: blur(10px);
    }
    .stack {
      display: grid;
      gap: 14px;
    }
    .tabs {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin: 0 0 18px;
      padding: 0;
      list-style: none;
    }
    .topbar button,
    button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border-radius: 10px;
      border: 1px solid var(--line);
      padding: 8px 12px;
      font: inherit;
      font-size: 0.86rem;
      font-weight: 500;
      background: rgba(255, 255, 255, 0.96);
      color: var(--ink);
      cursor: pointer;
      transition: background 120ms ease, border-color 120ms ease, box-shadow 120ms ease;
    }
    .topbar button:hover, button:hover {
      border-color: var(--line-strong);
      background: #fff;
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
    }
    button[type="submit"] {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
      box-shadow: 0 10px 24px rgba(15, 23, 42, 0.14);
    }
    button[type="submit"]:hover {
      background: #0f172a;
      border-color: #0f172a;
    }
    .stats {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      margin: 0 0 16px;
      color: var(--muted);
      font-size: 0.9rem;
    }
    .stats > div {
      padding: 8px 12px;
      border: 1px solid var(--line);
      border-radius: 999px;
      background: rgba(255, 255, 255, 0.72);
    }
    label {
      display: block;
      font-size: 0.8rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 6px;
    }
    select, input[type="file"], input[type="text"], textarea, button {
      width: 100%;
      border-radius: 12px;
      border: 1px solid var(--line);
      padding: 11px 13px;
      font: inherit;
      background: rgba(255, 255, 255, 0.98);
      color: var(--ink);
    }
    select:focus, input[type="text"]:focus, textarea:focus {
      outline: none;
      border-color: #c7d2fe;
      box-shadow: 0 0 0 4px rgba(99, 102, 241, 0.08);
    }
    textarea {
      min-height: 240px;
      resize: vertical;
      line-height: 1.6;
    }
    .notice {
      padding: 11px 14px;
      border: 1px solid var(--line);
      border-radius: 12px;
      margin-bottom: 14px;
      font-size: 0.92rem;
      background: var(--ok-bg);
    }
    .notice.error {
      color: var(--error);
      background: var(--error-bg);
      border-color: #f3d0cc;
    }
    ul.files {
      list-style: none;
      padding: 0;
      margin: 14px 0 0;
      display: grid;
      gap: 10px;
    }
    ul.files li {
      display: flex;
      gap: 12px;
      align-items: flex-start;
      padding: 14px 16px;
      border: 1px solid rgba(229, 231, 235, 0.9);
      border-radius: 14px;
      background: var(--surface-soft);
    }
    .actions {
      margin-top: 14px;
    }
    .capture-head button,
    .capture-actions button {
      width: auto;
    }
    .capture-actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    @media (max-width: 640px) {
      .panel { padding: 16px; }
    }
  </style>
</head>
<body>
` + sharedWebShellHeaderHTML + `
  <main>
    <p class="lead">One workflow at a time: quick capture, file upload, existing source linking, or staged review.</p>
    {{if .Status}}<div class="notice ok">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}
    <div class="stats">
      <div>Imported sources: <strong>{{.ImportedCount}}</strong></div>
      <div>Staged files: <strong>{{.StagedCount}}</strong></div>
    </div>
    <ul class="tabs">
      {{range .Nav}}
      <li><a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a></li>
      {{end}}
    </ul>

    <section class="section">
      {{if .IsPasteView}}
      <div class="panel">
      <h2>Capture Notes</h2>
      <p class="meta">Paste markdown notes directly. Pick a theme or issue now if you already know where this source belongs.</p>
      <form method="post" action="/paste">
        <div class="stack">
          <div>
            <label for="filename">Name</label>
            <input id="filename" type="text" name="filename" placeholder="meeting-notes">
          </div>
          <div>
            <label for="markdown">Markdown</label>
            <textarea id="markdown" name="markdown" placeholder="# Notes&#10;&#10;Paste markdown here." required></textarea>
          </div>
          <div>
            <label for="paste-theme">Theme</label>
            <select id="paste-theme" name="theme_id">
              <option value="">Leave unlinked</option>
              {{range .Themes}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="paste-issue">Issue</label>
            <select id="paste-issue" name="issue_id">
              <option value="">Leave unlinked</option>
              {{range .Issues}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <button type="submit">Capture Markdown</button>
          </div>
        </div>
        <p class="meta">If no name is provided, Workbench uses a random ID. If a name is provided, it saves as <code>&lt;slug&gt;--&lt;id&gt;.md</code>.</p>
      </form>
      </div>
      {{else if .IsUploadView}}
      <div class="panel">
      <h2>Upload File</h2>
      <p class="meta">Drop or pick a file to add it. Markdown files are saved directly as source documents; other files stay staged for later agent work.</p>
      <form method="post" action="/upload" enctype="multipart/form-data">
        <div class="stack">
          <div>
            <label for="file">File</label>
            <input id="file" type="file" name="file" accept=".md,.markdown,text/markdown,.txt,.text,.csv,.tsv,.docx,.pptx,.xlsx" required>
          </div>
          <div>
            <label for="upload-theme">Theme</label>
            <select id="upload-theme" name="theme_id">
              <option value="">Leave unlinked</option>
              {{range .Themes}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="upload-issue">Issue</label>
            <select id="upload-issue" name="issue_id">
              <option value="">Leave unlinked</option>
              {{range .Issues}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <button type="submit">Stage Upload</button>
          </div>
        </div>
        <p class="meta">Supported file types include <code>.md</code>, <code>.markdown</code>, <code>.txt</code>, <code>.csv</code>, <code>.docx</code>, <code>.pptx</code>, and <code>.xlsx</code>.</p>
      </form>
      </div>
      {{else if .IsLinkView}}
      <div class="panel">
      <h2>Link Existing Source</h2>
      <p class="meta">Use this when the source document already exists and you only need to associate it with a theme or issue.</p>
      <form method="post" action="/link">
        <div class="stack">
          <div>
            <label for="source-ref">Source document</label>
            <select id="source-ref" name="source_ref" required>
              <option value="">Choose a source document</option>
              {{range .SourceDocuments}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="link-theme">Theme</label>
            <select id="link-theme" name="theme_id">
              <option value="">Do not link to a theme</option>
              {{range .Themes}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="link-issue">Issue</label>
            <select id="link-issue" name="issue_id">
              <option value="">Do not link to an issue</option>
              {{range .Issues}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <button type="submit">Link Source Document</button>
          </div>
        </div>
      </form>
      {{if .SourceDocuments}}
      <ul class="files">
        {{range .SourceDocuments}}
        <li>
          <div>
            <div>{{.Label}}</div>
            <div class="meta"><code>{{.Value}}</code></div>
          </div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="empty">No source documents yet.</p>
      {{end}}
      </div>
      {{else if .IsStagedView}}
      <div class="panel">
      <h2>Staged Files</h2>
      <p class="meta">Files here are waiting for later agent work or review.</p>
      {{if .StagedItems}}
      <ul class="files">
        {{range .StagedItems}}
        <li>
          <div>
            <div>{{.Name}}</div>
            <div class="meta">Staged in <code>sources/files/staged/</code>. Extract this later with an agent.</div>
            {{if .ThemeLabel}}<div class="meta">Theme: {{.ThemeLabel}}</div>{{end}}
            {{if .IssueLabel}}<div class="meta">Issue: {{.IssueLabel}}</div>{{end}}
          </div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="empty">No staged files yet.</p>
      {{end}}
      </div>
      {{end}}
    </section>
` + sharedCaptureModalHTML + `
  </main>
` + sharedCaptureModalScript + `
</body>
</html>`

const eventWorkbenchHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Events · Workbench</title>
  <style>
    :root {
      --bg: #f5f5f7;
      --surface: #ffffff;
      --surface-soft: #fafafa;
      --ink: #111827;
      --muted: #6b7280;
      --line: #e5e7eb;
      --accent: #111827;
      --error: #b42318;
      --error-bg: #fef3f2;
      --content-width: 1480px;
      --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 12px 30px rgba(15, 23, 42, 0.05);
    }
` + sharedWebShellCSS + `
    main {
      flex: 1 1 auto;
      min-height: 0;
      padding: 0 20px 20px;
      display: grid;
      gap: 18px;
    }
` + sharedEventPageCSS + `
    button {
      border: 1px solid var(--line);
      background: var(--surface);
      color: var(--ink);
      border-radius: 999px;
      padding: 9px 14px;
      cursor: pointer;
    }
    .panel-head { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; flex-wrap: wrap; }
    .panel-actions { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
    .event-list { display: grid; gap: 12px; align-content: start; }
    .event-row { display: grid; gap: 6px; align-content: start; padding: 14px; border: 1px solid var(--line); border-radius: 14px; background: var(--surface-soft); }
    .event-row a { color: var(--ink); text-decoration: none; font-weight: 600; }
  </style>
</head>
<body>
` + sharedWebShellHeaderHTML + `
  <main>
    {{if .Status}}<div class="message">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="message error">{{.Error}}</div>{{end}}
      <section class="panel stack">
        <div class="panel-head">
          <div>
            <h2 style="margin:0 0 6px;">{{.CurrentTitle}}</h2>
            <div class="count">{{.CurrentCountLabel}}</div>
          </div>
          <div class="panel-actions">
            <a class="toolbar-button" href="{{.NewEventHref}}">New Event</a>
          </div>
        </div>
        {{if .Entries}}
        <div class="event-list">
          {{range .Entries}}
          <article class="event-row">
            <a href="{{.Href}}">{{.Title}}</a>
            <div class="meta">{{.ThemeLabel}}{{if .Updated}} · Updated {{.Updated}}{{end}}</div>
          </article>
          {{end}}
        </div>
        {{else}}
        <div class="empty">No events yet.</div>
        {{end}}
      </section>
` + sharedCaptureModalHTML + `
  </main>
` + sharedCaptureModalScript + `
</body>
</html>`

const eventCreateHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>New Event · Workbench</title>
  <style>
    :root {
      --bg: #f5f5f7;
      --surface: #ffffff;
      --surface-soft: #fafafa;
      --ink: #111827;
      --muted: #6b7280;
      --line: #e5e7eb;
      --accent: #111827;
      --error: #b42318;
      --error-bg: #fef3f2;
      --content-width: 1480px;
      --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 12px 30px rgba(15, 23, 42, 0.05);
    }
` + sharedWebShellCSS + `
    main {
      flex: 1 1 auto;
      min-height: 0;
      padding: 0 20px 20px;
      display: grid;
      gap: 18px;
    }
` + sharedEventPageCSS + `
    button {
      border: 1px solid var(--line);
      background: var(--surface);
      color: var(--ink);
      border-radius: 999px;
      padding: 9px 14px;
      cursor: pointer;
    }
    .create-layout { display: grid; gap: 18px; grid-template-columns: minmax(0, 1.6fr) minmax(280px, 420px); align-items: start; }
    .compose-panel { gap: 18px; }
    .compose-header h2 { margin: 0 0 8px; }
    .compose-header p { margin: 0; color: var(--muted); line-height: 1.55; }
    .compose-form { display: grid; gap: 18px; }
    .compose-form textarea { min-height: 420px; resize: vertical; }
    .form-section { display: grid; gap: 8px; }
    .form-actions { display: flex; justify-content: space-between; gap: 12px; align-items: center; flex-wrap: wrap; }
    .side-panel h3 { margin: 0 0 6px; font-size: 1rem; }
    .side-panel p { margin: 0; color: var(--muted); }
    .event-list { display: grid; gap: 12px; }
    .event-row { display: grid; gap: 6px; padding: 14px; border: 1px solid var(--line); border-radius: 14px; background: var(--surface-soft); }
    .event-row a { color: var(--ink); text-decoration: none; font-weight: 600; }
    @media (max-width: 920px) { .create-layout { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
` + sharedWebShellHeaderHTML + `
  <main>
    {{if .Status}}<div class="message">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="message error">{{.Error}}</div>{{end}}
    <div class="create-layout">
      <section class="panel compose-panel">
        <div>
          <div class="compose-header">
            <h2>New Event</h2>
            <p>Create an event record for a meeting, sync, incident, or ad-hoc conversation. This page is focused on registration first, like a compose screen.</p>
          </div>
        </div>
        <form method="post" action="{{.CreateAction}}" class="compose-form">
          <div class="form-section">
            <label for="event-title">Title</label>
            <input id="event-title" type="text" name="title" placeholder="Weekly sync" required autofocus>
          </div>
          <div class="form-section">
            <label for="event-theme">Theme</label>
            <select id="event-theme" name="theme_id">
              {{range .Themes}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
            </select>
          </div>
          <div class="form-section">
            <label for="event-body">Notes</label>
            <textarea id="event-body" name="body" placeholder="# Agenda or notes"></textarea>
          </div>
          <div class="form-actions">
            <a href="{{.EventsHref}}">Back to events</a>
            <button type="submit">Create Event</button>
          </div>
        </form>
      </section>
      <aside class="panel stack side-panel">
        <div>
          <h3>{{if .SelectedThemeTitle}}{{.SelectedThemeTitle}} Events{{else}}Recent Events{{end}}</h3>
          <div class="count">{{.CurrentCountLabel}}</div>
        </div>
        <p>{{if .SelectedThemeTitle}}Use the selected theme when this event belongs to an existing thread.{{else}}Leave Theme as Global when the event does not belong to a theme yet.{{end}}</p>
        {{if .Entries}}
        <div class="event-list">
          {{range .Entries}}
          <article class="event-row">
            <a href="{{.Href}}">{{.Title}}</a>
            <div class="meta">{{.ThemeLabel}}{{if .Updated}} · Updated {{.Updated}}{{end}}</div>
          </article>
          {{end}}
        </div>
        {{else}}
        <div class="empty">No events yet.</div>
        {{end}}
      </aside>
    </div>
` + sharedCaptureModalHTML + `
  </main>
` + sharedCaptureModalScript + `
</body>
</html>`

const eventWorkspaceHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} · Events</title>
  <style>
    :root {
      --bg: #f5f5f7;
      --surface: #ffffff;
      --surface-soft: #fafafa;
      --ink: #111827;
      --muted: #6b7280;
      --line: #e5e7eb;
      --error: #b42318;
      --error-bg: #fef3f2;
      --content-width: 1480px;
      --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 12px 30px rgba(15, 23, 42, 0.05);
    }
` + sharedWebShellCSS + `
    main {
      flex: 1 1 auto;
      min-height: 0;
      padding: 0 20px 20px;
      display: grid;
      gap: 18px;
    }
` + sharedEventPageCSS + `
    button {
      border: 1px solid var(--line);
      background: var(--surface);
      color: var(--ink);
      border-radius: 999px;
      padding: 9px 14px;
      cursor: pointer;
    }
    textarea { min-height: 420px; resize: vertical; }
    .panel-head { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; flex-wrap: wrap; }
    .form-actions { display: flex; justify-content: flex-end; gap: 12px; }
  </style>
</head>
<body>
` + sharedWebShellHeaderHTML + `
  <main>
    {{if .Status}}<div class="message">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="message error">{{.Error}}</div>{{end}}
    <section class="panel">
      <div class="panel-head">
        <div>
          <div class="meta">{{.ThemeLabel}}{{if .Updated}} · Updated {{.Updated}}{{end}}</div>
        </div>
        <a class="toolbar-button" href="{{.ReturnHref}}">{{.ReturnLabel}}</a>
      </div>
      <form method="post" action="{{.SaveAction}}" style="display:grid; gap:14px;">
        <div>
          <label for="event-title">Title</label>
          <input id="event-title" type="text" name="title" value="{{.Title}}" required>
        </div>
        <div>
          <label for="event-theme">Theme</label>
          <select id="event-theme" name="theme_id">
            {{range .Themes}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
          </select>
        </div>
        <div>
          <label for="event-body">Notes</label>
          <textarea id="event-body" name="body" placeholder="# Notes">{{.MainBody}}</textarea>
        </div>
        <div class="form-actions"><button class="toolbar-button" type="submit">Save Event</button></div>
      </form>
    </section>
` + sharedCaptureModalHTML + `
  </main>
` + sharedCaptureModalScript + `
</body>
</html>`

const workItemWorkspaceHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} · Workbench</title>
  <style>
    :root {
      --bg: #f5f5f7;
      --surface: #ffffff;
      --surface-soft: #fafafa;
      --surface-muted: #f8fafc;
      --ink: #111827;
      --muted: #6b7280;
      --line: #e5e7eb;
      --line-strong: #d1d5db;
      --accent: #111827;
      --accent-soft: #eef2f7;
      --error: #b42318;
      --error-bg: #fef3f2;
      --content-inset: 18px;
      --content-width: 1480px;
      --sidebar-expanded-width: 280px;
      --pane-header-height: 58px;
      --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 12px 30px rgba(15, 23, 42, 0.05);
      --shadow-popover: 0 18px 40px rgba(15, 23, 42, 0.12);
    }
` + sharedWebShellCSS + `
    body { height: 100dvh; overflow: hidden; }
    main {
      flex: 1 1 auto;
      min-height: 0;
      display: flex;
      flex-direction: column;
      padding: 0 20px 20px;
      overflow: hidden;
    }
    h1, h2, h3 {
      margin: 0;
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    h1 { font-size: 1.5rem; }
    h2 { font-size: 1rem; margin-bottom: 12px; }
    h3 { font-size: 0.92rem; margin-bottom: 8px; }
    p.lead, .meta {
      color: var(--muted);
      font-size: 0.92rem;
      line-height: 1.6;
    }
    .workspace {
      display: grid;
      gap: 16px;
      grid-template-columns: var(--sidebar-expanded-width) minmax(0, 1fr);
      align-items: stretch;
      flex: 1 1 auto;
      min-height: 0;
      height: 100%;
      overflow: hidden;
    }
    .workspace[data-sidebar-collapsed="true"] {
      grid-template-columns: 56px minmax(0, 1fr);
    }
    .agent-pane,
    .workspace-main {
      border: 1px solid rgba(229, 231, 235, 0.9);
      border-radius: 20px;
      background: rgba(255, 255, 255, 0.88);
      box-shadow: var(--shadow);
      backdrop-filter: blur(10px);
    }
    .agent-pane {
      display: flex;
      flex-direction: column;
      min-width: 0;
      min-height: 0;
      height: 100%;
      overflow: auto;
    }
    .sidebar-toolbar {
      display: flex;
      justify-content: flex-start;
      align-items: center;
      gap: 10px;
      min-height: var(--pane-header-height);
      padding: 12px;
      border-bottom: 1px solid rgba(229, 231, 235, 0.8);
      position: sticky;
      top: 0;
      background: rgba(255, 255, 255, 0.72);
      backdrop-filter: blur(8px);
      z-index: 1;
    }
    .sidebar-title,
    .section-label {
      font-size: 0.74rem;
      font-weight: 600;
      color: var(--muted);
      letter-spacing: 0.06em;
      text-transform: uppercase;
    }
    .sidebar-section {
      padding: 14px var(--content-inset);
      border-top: 1px solid rgba(229, 231, 235, 0.8);
      min-width: 0;
    }
    .sidebar-section:first-child { border-top: 0; }
    .sidebar-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      margin-bottom: 10px;
    }
    .mode-toggle,
    .save-button,
    .capture-actions button,
    .capture-head button,
    button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border-radius: 10px;
      border: 1px solid var(--line);
      padding: 8px 12px;
      font: inherit;
      font-size: 0.86rem;
      font-weight: 500;
      background: rgba(255, 255, 255, 0.96);
      color: var(--ink);
      cursor: pointer;
      transition: background 120ms ease, border-color 120ms ease, box-shadow 120ms ease;
    }
    .mode-toggle:hover,
    .save-button:hover,
    .capture-actions button:hover,
    .capture-head button:hover,
    button:hover {
      border-color: var(--line-strong);
      background: #fff;
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
    }
    .save-button {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
      box-shadow: 0 10px 24px rgba(15, 23, 42, 0.14);
    }
    .save-button:hover {
      background: #0f172a;
      border-color: #0f172a;
    }
    input[type="text"] {
      width: 100%;
      border-radius: 12px;
      border: 1px solid var(--line);
      padding: 11px 13px;
      font: inherit;
      background: rgba(255, 255, 255, 0.98);
      color: var(--ink);
    }
    .sidebar-toolbar button,
    .mode-toggle,
    .save-button,
    .capture-head button,
    .capture-actions button {
      width: auto;
      min-width: 0;
    }
    .sidebar-toggle {
      width: 32px;
      min-width: 32px;
      height: 32px;
      padding: 0;
      flex: 0 0 32px;
      font-size: 14px;
      line-height: 1;
      border-radius: 9px;
      box-shadow: none;
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    .editor-stack {
      display: flex;
      flex-direction: column;
      gap: 16px;
      flex: 1;
      min-height: 0;
      background: linear-gradient(180deg, rgba(248, 250, 252, 0.55) 0%, rgba(255, 255, 255, 0) 100%);
    }
    .workspace-main {
      display: flex;
      flex-direction: column;
      min-width: 0;
      min-height: 0;
      height: 100%;
      overflow: hidden;
    }
    .workspace-main form {
      display: flex;
      flex: 1;
      min-height: 0;
      overflow: hidden;
    }
    .editor-only {
      display: flex;
      flex: 1;
      min-height: 0;
      flex-direction: column;
    }
    .editor-stack[data-mode="editor"] .preview-panel,
    .editor-stack[data-mode="preview"] .editor-only {
      display: none;
    }
    .preview-panel {
      display: flex;
      flex: 1;
      min-height: 0;
      flex-direction: column;
      overflow: hidden;
    }
    .editor-stack[data-mode="preview"] .preview-panel {
      display: flex;
    }
    .tabs, .list, .tree-list {
      list-style: none;
      padding: 0;
      margin: 0;
    }
    .tabs {
      display: flex;
      gap: 8px;
    }
    .list a, .tree-list a { color: inherit; text-decoration: none; }
    .list li {
      border-top: 1px solid rgba(229, 231, 235, 0.8);
    }
    .list li:first-child { border-top: 0; }
    .list a {
      display: block;
      padding: 12px 0;
    }
    .list a.active { font-weight: 600; }
    .list .meta {
      margin-top: 4px;
      font-size: 0.84rem;
    }
    .tree-list {
      display: grid;
      gap: 4px;
    }
    .tree-list a,
    .tree-list .active-item {
      display: block;
      padding: 9px 10px;
      border-radius: 12px;
      font-size: 0.9rem;
    }
    .tree-list a.active,
    .tree-list .active-item {
      background: var(--accent-soft);
      font-weight: 600;
    }
    .tree-meta {
      margin-top: 3px;
      color: var(--muted);
      font-size: 0.82rem;
      word-break: break-word;
    }
    .sidebar-preview {
      margin-top: 12px;
      padding-top: 12px;
      border-top: 1px solid rgba(229, 231, 235, 0.8);
    }
    .workspace[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-title,
    .workspace[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) #agent-pane-content {
      display: none;
    }
    .workspace[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-toolbar {
      border-bottom: 0;
    }
    .workspace[data-sidebar-collapsed="true"][data-sidebar-hovered="true"] .agent-pane {
      width: min(var(--sidebar-expanded-width), calc(100vw - 32px));
      z-index: 3;
      box-shadow: var(--shadow-popover);
    }
    textarea {
      width: 100%;
      min-height: 0;
      flex: 1;
      resize: none;
      border: 0;
      border-radius: 0;
      padding: 18px var(--content-inset);
      font: inherit;
      line-height: 1.65;
      background: transparent;
      color: var(--ink);
    }
    textarea:focus { outline: none; }
    .preview-panel {
      border-top: 1px solid rgba(229, 231, 235, 0.8);
      padding-top: 10px;
    }
    .editor-stack[data-mode="preview"] .preview-panel {
      border-top: 0;
      padding-top: 0;
    }
    .preview-surface {
      border: 0;
      padding: 18px var(--content-inset);
      min-height: 0;
      flex: 1;
      height: 100%;
      background: transparent;
      overflow: auto;
      line-height: 1.7;
    }
    .preview-surface img {
      max-width: 100%;
      height: auto;
      border-radius: 12px;
    }
    .preview-surface pre {
      overflow: auto;
      padding: 12px 14px;
      border-radius: 12px;
      background: var(--surface-soft);
      border: 1px solid rgba(229, 231, 235, 0.8);
    }
    .preview-surface code,
    pre.viewer {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    .mode-actions {
      display: flex;
      align-items: center;
      gap: 10px;
      justify-content: space-between;
      min-height: var(--pane-header-height);
      padding: 12px var(--content-inset);
      border-bottom: 1px solid rgba(229, 231, 235, 0.8);
      background: rgba(255, 255, 255, 0.72);
      backdrop-filter: blur(8px);
    }
    .mode-toggle-group {
      display: inline-flex;
      align-items: center;
      gap: 0;
      border: 1px solid var(--line);
      border-radius: 12px;
      overflow: hidden;
      background: rgba(255, 255, 255, 0.96);
    }
    .mode-toggle {
      margin-left: 0;
      border: 0;
      border-right: 1px solid var(--line);
      border-radius: 0;
      background: transparent;
      color: var(--muted);
      box-shadow: none;
    }
    .mode-toggle:last-child { border-right: 0; }
    .mode-toggle[aria-pressed="true"] {
      background: var(--accent-soft);
      color: var(--ink);
      font-weight: 600;
    }
    .save-button { margin: 0; }
    #work-item-save-button[hidden] { display: none; }
    .mode-actions-right {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 10px;
      margin-left: auto;
      min-width: 0;
      flex-wrap: wrap;
    }
    .capture-actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    .workspace-title {
      padding-left: var(--content-inset);
    }
    pre.viewer {
      margin: 0;
      white-space: pre-wrap;
      word-break: break-word;
      font-size: 0.88rem;
      line-height: 1.6;
    }
    .empty {
      color: var(--muted);
      font-size: 0.92rem;
    }
    .hint {
      color: var(--muted);
      font-size: 0.84rem;
    }
    .editor-feedback {
      display: none;
      padding: 8px 10px;
      border-radius: 999px;
      font-size: 0.82rem;
      width: auto;
      max-width: min(480px, 100%);
      border: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.9);
    }
    .editor-feedback.error {
      display: inline-flex;
      color: var(--error);
      background: var(--error-bg);
      border-color: #f3d0cc;
    }
    .editor-feedback.success {
      display: inline-flex;
      color: #0f6b46;
      background: #f2fbf6;
      border-color: #cfe9d9;
    }
    @media (max-width: 920px) {
      .shell-header { padding: 14px 14px 10px; }
      main { padding: 0 14px 14px; }
    }
    @media (max-width: 720px) {
      .shell-header { padding: 14px 14px 10px; }
      textarea { min-height: 320px; }
      .preview-surface { min-height: 320px; }
    }
  </style>
</head>
<body>
` + sharedWebShellHeaderHTML + `
  <main>
    <div class="workspace" data-sidebar-collapsed="false" data-sidebar-hovered="false">
      <aside id="agent-pane" class="agent-pane" data-refresh-url="{{.AgentRefreshHref}}">
        <div class="sidebar-toolbar">
          <button id="toggle-sidebar" class="sidebar-toggle" type="button" aria-expanded="true" aria-controls="agent-pane" title="Toggle sidebar">&#9664;</button>
          <div class="sidebar-title">Explorer</div>
        </div>
        <div id="agent-pane-content">{{.AgentPaneHTML}}</div>
      </aside>
      <section class="workspace-main">
        <div class="mode-actions">
          <div class="section-label">Main</div>
          <div class="mode-actions-right">
            <div id="editor-feedback" class="editor-feedback" role="status" aria-live="polite"></div>
            <button id="work-item-save-button" class="save-button" type="submit" form="work-item-editor">Save</button>
            <div class="mode-toggle-group" role="group" aria-label="Editor mode">
              <button id="toggle-edit-mode" class="mode-toggle" type="button" aria-pressed="true">Edit</button>
              <button id="toggle-preview-mode" class="mode-toggle" type="button" aria-pressed="false">Preview</button>
            </div>
          </div>
        </div>
        <form id="work-item-editor" method="post" action="{{.SaveAction}}" data-preview-url="{{.PreviewAction}}" data-asset-upload-url="{{.AssetUploadAction}}">
          <div class="editor-stack" data-mode="editor">
            <div class="editor-only stack">
              <textarea id="work-item-body" name="body" placeholder="# Notes">{{.MainBody}}</textarea>
            </div>
            <div class="preview-panel stack">
              <div id="main-preview" class="preview-surface" tabindex="0">{{.MainPreviewHTML}}</div>
            </div>
          </div>
        </form>
      </section>
    </div>
` + sharedCaptureModalHTML + `
  </main>
  <script>
    (() => {
      const form = document.getElementById("work-item-editor");
      const editorStack = form ? form.querySelector(".editor-stack") : null;
      const textarea = document.getElementById("work-item-body");
      const preview = document.getElementById("main-preview");
      const feedback = document.getElementById("editor-feedback");
      const toggleEditButton = document.getElementById("toggle-edit-mode");
      const togglePreviewButton = document.getElementById("toggle-preview-mode");
      const saveButton = document.getElementById("work-item-save-button");
      const workspace = document.querySelector(".workspace");
      const toggleSidebarButton = document.getElementById("toggle-sidebar");
      const agentPane = document.getElementById("agent-pane");
      const agentPaneContent = document.getElementById("agent-pane-content");
      const sidebarStateKey = "workbench.sidebar.collapsed";
` + sharedCaptureModalSetupJS + `
      const previewAction = form ? form.dataset.previewUrl : "";
      const assetUploadAction = form ? form.dataset.assetUploadUrl : "";
      let saveTimer = null;
      const setFeedback = (message, tone) => {
        if (!feedback) {
          return;
        }
        feedback.textContent = message || "";
        feedback.className = message ? "editor-feedback " + (tone || "error") : "editor-feedback";
      };
      const showSavedFeedback = (message) => {
        setFeedback(message || "saved work item document", "success");
        if (saveTimer) {
          window.clearTimeout(saveTimer);
        }
        saveTimer = window.setTimeout(() => setFeedback("", ""), 1500);
      };
      const syncPreviewViewportHeight = () => {
        if (!preview || !form) {
          return;
        }
        if (previewMode() !== "preview") {
          preview.style.height = "";
          preview.style.maxHeight = "";
          return;
        }
        const rect = preview.getBoundingClientRect();
        const rootRect = form.getBoundingClientRect();
        const available = Math.max(160, Math.floor(rootRect.bottom - rect.top));
        preview.style.height = available + "px";
        preview.style.maxHeight = available + "px";
      };
      const sidebarCollapsed = () => workspace && workspace.dataset.sidebarCollapsed === "true";
      const syncSidebarState = () => {
        if (!workspace || !toggleSidebarButton) {
          return;
        }
        const collapsed = sidebarCollapsed();
        const hovered = workspace.dataset.sidebarHovered === "true";
        const expanded = !collapsed || hovered;
        toggleSidebarButton.setAttribute("aria-expanded", expanded ? "true" : "false");
        toggleSidebarButton.innerHTML = expanded ? "&#9664;" : "&#9654;";
      };
      const setSidebarCollapsed = (collapsed) => {
        if (!workspace) {
          return;
        }
        workspace.dataset.sidebarCollapsed = collapsed ? "true" : "false";
        window.localStorage.setItem(sidebarStateKey, collapsed ? "true" : "false");
        if (!collapsed) {
          workspace.dataset.sidebarHovered = "false";
        }
        syncSidebarState();
      };
      const setSidebarHovered = (hovered) => {
        if (!workspace) {
          return;
        }
        workspace.dataset.sidebarHovered = sidebarCollapsed() && hovered ? "true" : "false";
        syncSidebarState();
      };
      const saveDocument = async (options = {}) => {
        if (!form) {
          return false;
        }
        setFeedback("", "");
        try {
          const response = await fetch(form.action, {
            method: "POST",
            headers: {
              "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
              "X-Requested-With": "fetch"
            },
            body: new URLSearchParams(new FormData(form)).toString()
          });
          const payload = await response.json().catch(() => ({}));
          if (!response.ok) {
            throw new Error(payload && payload.error ? payload.error : "save failed");
          }
          showSavedFeedback(payload && payload.status ? payload.status : "saved work item document");
          if (options.openPreview) {
            await setPreviewMode("preview");
          }
          return true;
        } catch (error) {
          setFeedback(error && error.message ? error.message : "save failed", "error");
          return false;
        }
      };
      const previewMode = () => editorStack ? editorStack.dataset.mode || "editor" : "editor";
      const setPreviewMode = async (mode, options = {}) => {
        if (!editorStack) {
          return;
        }
        editorStack.dataset.mode = mode === "preview" ? "preview" : "editor";
        if (saveButton) {
          saveButton.hidden = editorStack.dataset.mode === "preview";
          saveButton.setAttribute("aria-hidden", saveButton.hidden ? "true" : "false");
        }
        if (toggleEditButton && togglePreviewButton) {
          const previewActive = editorStack.dataset.mode === "preview";
          toggleEditButton.setAttribute("aria-pressed", previewActive ? "false" : "true");
          togglePreviewButton.setAttribute("aria-pressed", previewActive ? "true" : "false");
        }
        if (editorStack.dataset.mode === "preview") {
          await refreshPreview();
          window.requestAnimationFrame(syncPreviewViewportHeight);
          if (!options.skipFocus && preview) {
            preview.focus();
          }
          return;
        }
        syncPreviewViewportHeight();
        if (!options.skipFocus && textarea) {
          textarea.focus();
        }
      };
      const normalizedSearchIndex = (value) => {
        const text = String(value || "");
        const chars = [];
        const offsets = [];
        let spaced = false;
        for (let i = 0; i < text.length; i += 1) {
          const ch = text[i];
          if (/\s/.test(ch)) {
            if (!chars.length || spaced) {
              continue;
            }
            chars.push(" ");
            offsets.push(i);
            spaced = true;
            continue;
          }
          chars.push(ch.toLowerCase());
          offsets.push(i);
          spaced = false;
        }
        while (chars.length && chars[chars.length - 1] === " ") {
          chars.pop();
          offsets.pop();
        }
        return { text: chars.join(""), offsets, sourceLength: text.length };
      };
      const sourceOffsetFromNormalizedIndex = (index, normalized) => {
        if (!normalized) {
          return -1;
        }
        if (index <= 0) {
          return 0;
        }
        if (index >= normalized.offsets.length) {
          return normalized.sourceLength;
        }
        return normalized.offsets[index];
      };
      const resolveTextOffset = (value, haystackValue, baseOffset, relativeIndex = 0) => {
        const needle = normalizedSearchIndex(value);
        if (!needle.text) {
          return -1;
        }
        const haystack = normalizedSearchIndex(haystackValue);
        const index = haystack.text.indexOf(needle.text);
        if (index < 0) {
          return -1;
        }
        const clamped = Math.max(0, Math.min(relativeIndex, needle.text.length));
        return baseOffset + sourceOffsetFromNormalizedIndex(index + clamped, haystack);
      };
      const findTextOffset = (value, relativeIndex = 0) => {
        if (!textarea) {
          return -1;
        }
        return resolveTextOffset(value, textarea.value, 0, relativeIndex);
      };
      const caretPointFromEvent = (event) => {
        if (document.caretPositionFromPoint) {
          const position = document.caretPositionFromPoint(event.clientX, event.clientY);
          if (position) {
            return { node: position.offsetNode, offset: position.offset };
          }
        }
        if (document.caretRangeFromPoint) {
          const range = document.caretRangeFromPoint(event.clientX, event.clientY);
          if (range) {
            return { node: range.startContainer, offset: range.startOffset };
          }
        }
        return null;
      };
      const blockTextOffsetFromEvent = (block, event) => {
        if (!block) {
          return -1;
        }
        const caretPoint = caretPointFromEvent(event);
        if (!caretPoint || !caretPoint.node || !block.contains(caretPoint.node)) {
          return -1;
        }
        const range = document.createRange();
        range.selectNodeContents(block);
        try {
          range.setEnd(caretPoint.node, caretPoint.offset);
        } catch (_) {
          return -1;
        }
        return normalizedSearchIndex(range.toString()).text.length;
      };
      const blockSourceRange = (block) => {
        if (!block || !block.dataset) {
          return null;
        }
        const start = Number.parseInt(block.dataset.sourceStart || "", 10);
        const end = Number.parseInt(block.dataset.sourceEnd || "", 10);
        if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) {
          return null;
        }
        return { start, end };
      };
      const focusEditorAt = async (offset) => {
        await setPreviewMode("editor", { skipFocus: true });
        if (!textarea) {
          return;
        }
        const caret = Math.max(0, offset);
        textarea.focus();
        textarea.selectionStart = caret;
        textarea.selectionEnd = caret;
        const lineHeight = parseFloat(window.getComputedStyle(textarea).lineHeight) || 20;
        const lines = textarea.value.slice(0, caret).split("\n").length - 1;
        textarea.scrollTop = Math.max(0, (lines - 2) * lineHeight);
      };
      if (form) {
        form.addEventListener("submit", async (event) => {
          event.preventDefault();
          await saveDocument();
        });
        document.addEventListener("keydown", (event) => {
          const tag = event.target && event.target.tagName ? String(event.target.tagName).toLowerCase() : "";
          const editable = tag === "input" || tag === "textarea" || tag === "select" || event.target && event.target.isContentEditable;
          if ((event.metaKey || event.ctrlKey) && !event.shiftKey && String(event.key).toLowerCase() === "s") {
            event.preventDefault();
            void saveDocument();
            return;
          }
          if ((event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "s") {
            event.preventDefault();
            void saveDocument({ openPreview: true });
            return;
          }
          if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) {
            return;
          }
          if (event.key !== "Escape") {
            return;
          }
          if (captureDialog && captureDialog.open) {
            return;
          }
          event.preventDefault();
          setPreviewMode(previewMode() === "preview" ? "editor" : "preview");
        });
      }
      let previewTimer = null;
      const refreshPreview = async () => {
        if (!textarea || !preview || !previewAction) {
          return;
        }
        try {
          const response = await fetch(previewAction, {
            method: "POST",
            headers: {
              "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
              "X-Requested-With": "fetch"
            },
            body: new URLSearchParams({ body: textarea.value }).toString()
          });
          if (!response.ok) {
            return;
          }
          preview.innerHTML = await response.text();
          window.requestAnimationFrame(syncPreviewViewportHeight);
        } catch (_) {
        }
      };
      const queuePreviewRefresh = () => {
        if (previewTimer) {
          window.clearTimeout(previewTimer);
        }
        previewTimer = window.setTimeout(refreshPreview, 200);
      };
      if (toggleEditButton) {
        toggleEditButton.addEventListener("click", () => {
          if (previewMode() !== "editor") {
            setPreviewMode("editor");
          }
        });
      }
      if (togglePreviewButton) {
        togglePreviewButton.addEventListener("click", () => {
          if (previewMode() !== "preview") {
            setPreviewMode("preview");
          }
        });
      }
      if (toggleSidebarButton) {
        toggleSidebarButton.addEventListener("click", () => {
          setSidebarCollapsed(!sidebarCollapsed());
        });
      }
      if (agentPane) {
        agentPane.addEventListener("mouseenter", () => setSidebarHovered(true));
        agentPane.addEventListener("mouseleave", () => setSidebarHovered(false));
      }
      const persistedSidebarState = window.localStorage.getItem(sidebarStateKey);
      if (persistedSidebarState === "true" || persistedSidebarState === "false") {
        workspace.dataset.sidebarCollapsed = persistedSidebarState;
      }
      syncSidebarState();
      if (preview) {
        window.addEventListener("resize", syncPreviewViewportHeight);
        preview.addEventListener("dblclick", async (event) => {
          const block = event.target && event.target.closest ? event.target.closest("[data-source-start]") : null;
          if (block) {
            const sourceRange = blockSourceRange(block);
            const offset = sourceRange && textarea
              ? resolveTextOffset(block.textContent, textarea.value.slice(sourceRange.start, sourceRange.end), sourceRange.start, blockTextOffsetFromEvent(block, event))
              : findTextOffset(block.textContent, blockTextOffsetFromEvent(block, event));
            if (offset >= 0) {
              await focusEditorAt(offset);
              return;
            }
          }
          const selection = window.getSelection ? String(window.getSelection() || "") : "";
          const fallbackCandidates = [
            selection,
            event.target && event.target.textContent ? event.target.textContent : ""
          ];
          for (const candidate of fallbackCandidates) {
            const offset = findTextOffset(candidate);
            if (offset >= 0) {
              await focusEditorAt(offset);
              return;
            }
          }
          await focusEditorAt(textarea ? textarea.selectionStart || 0 : 0);
        });
      }
      if (textarea) {
        textarea.addEventListener("input", queuePreviewRefresh);
        const fileNameForBlob = (blob) => {
          const type = String(blob && blob.type || "").toLowerCase();
          if (type === "image/png") return "pasted-image.png";
          if (type === "image/jpeg") return "pasted-image.jpg";
          if (type === "image/gif") return "pasted-image.gif";
          if (type === "image/webp") return "pasted-image.webp";
          return "pasted-image.img";
        };
        const blobFromDataURL = (value) => {
          const match = /^data:(image\/[a-z0-9.+-]+);base64,(.+)$/i.exec(value || "");
          if (!match) {
            return null;
          }
          const binary = window.atob(match[2]);
          const bytes = new Uint8Array(binary.length);
          for (let i = 0; i < binary.length; i += 1) {
            bytes[i] = binary.charCodeAt(i);
          }
          return new Blob([bytes], { type: match[1] });
        };
        const extractPastedImageSync = (event) => {
          const clipboard = event.clipboardData;
          if (clipboard) {
            const items = Array.from(clipboard.items || []);
            const imageItem = items.find((item) => item.kind === "file" && String(item.type).startsWith("image/"));
            if (imageItem) {
              const file = imageItem.getAsFile();
              if (file) {
                return file;
              }
            }
            const files = Array.from(clipboard.files || []);
            const imageFile = files.find((file) => String(file.type).startsWith("image/"));
            if (imageFile) {
              return imageFile;
            }
            const html = clipboard.getData ? clipboard.getData("text/html") : "";
            const htmlMatch = /src=["'](data:image\/[^"']+)["']/i.exec(html || "");
            if (htmlMatch) {
              return blobFromDataURL(htmlMatch[1]);
            }
            const plain = clipboard.getData ? clipboard.getData("text/plain") : "";
            if (String(plain).startsWith("data:image/")) {
              return blobFromDataURL(plain);
            }
          }
          return null;
        };
        const extractPastedImageAsync = async () => {
          if (navigator.clipboard && typeof navigator.clipboard.read === "function") {
            try {
              const items = await navigator.clipboard.read();
              for (const item of items) {
                const imageType = item.types.find((type) => String(type).startsWith("image/"));
                if (imageType) {
                  return await item.getType(imageType);
                }
              }
            } catch (_) {
            }
          }
          return null;
        };
        const uploadPastedImage = async (blob) => {
          const formData = new FormData();
          formData.append("image", blob, fileNameForBlob(blob));
          const response = await fetch(assetUploadAction, {
            method: "POST",
            body: formData,
            headers: { "X-Requested-With": "fetch" }
          });
          if (!response.ok) {
            const text = await response.text();
            throw new Error(text || "image upload failed");
          }
          return response.json();
        };
        textarea.addEventListener("paste", async (event) => {
          if (!assetUploadAction) {
            return;
          }
          let blob = extractPastedImageSync(event);
          if (blob) {
            event.preventDefault();
          } else {
            blob = await extractPastedImageAsync();
          }
          if (!blob) {
            setFeedback("");
            return;
          }
          setFeedback("");
          try {
            const payload = await uploadPastedImage(blob);
            const insertion = payload.markdown || "";
            const start = textarea.selectionStart || 0;
            const end = textarea.selectionEnd || 0;
            const prefix = textarea.value.slice(0, start);
            const suffix = textarea.value.slice(end);
            const joiner = prefix && !prefix.endsWith("\n") ? "\n" : "";
            const trailer = suffix && !suffix.startsWith("\n") ? "\n" : "";
            textarea.value = prefix + joiner + insertion + trailer + suffix;
            const caret = (prefix + joiner + insertion).length;
            textarea.selectionStart = caret;
            textarea.selectionEnd = caret;
            textarea.focus();
            queuePreviewRefresh();
          } catch (error) {
            setFeedback(error && error.message ? error.message : "image paste failed");
          }
        });
      }

      if (!agentPane || !agentPaneContent || !agentPane.dataset.refreshUrl) {
        return;
      }
      let refreshing = false;
      const refreshAgentPane = async () => {
        if (refreshing || document.hidden) {
          return;
        }
        refreshing = true;
        try {
          const response = await fetch(agentPane.dataset.refreshUrl, {
            headers: { "X-Requested-With": "fetch" },
            cache: "no-store"
          });
          if (!response.ok) {
            return;
          }
          const html = await response.text();
          if (html !== agentPaneContent.innerHTML) {
            agentPaneContent.innerHTML = html;
          }
        } catch (_) {
        } finally {
          refreshing = false;
        }
      };
      window.setInterval(refreshAgentPane, 5000);
      document.addEventListener("visibilitychange", refreshAgentPane);
    })();
  </script>
</body>
</html>`

const workItemAgentPaneHTML = `
<section class="sidebar-section">
  <div class="sidebar-head">
    <div class="section-label">Main</div>
  </div>
  <ul class="tree-list">
    <li>
      <div class="active-item">{{.Title}}</div>
    </li>
  </ul>
</section>

<section class="sidebar-section">
  <div class="sidebar-head">
    <div class="section-label">Context</div>
    <ul class="tabs">
      <li><a href="{{.MemoRecentHref}}"{{if .IsMemoRecent}} class="active"{{end}}>Recent</a></li>
      <li><a href="{{.MemoTreeHref}}"{{if .IsMemoTree}} class="active"{{end}}>Tree</a></li>
    </ul>
  </div>
  {{if .Memos}}
  <ul class="tree-list">
    {{range .Memos}}
    <li>
      <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>
        <div>{{.Label}}</div>
        <div class="tree-meta">{{.Meta}}{{if .Modified}} · {{.Modified}}{{end}}</div>
      </a>
    </li>
    {{end}}
  </ul>
  {{if .SelectedMemoLabel}}
  <div class="sidebar-preview stack">
    <h3>{{.SelectedMemoLabel}}</h3>
    <pre class="viewer">{{.SelectedMemoBody}}</pre>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No context files yet.</p>
  {{end}}
</section>

<section class="sidebar-section">
  <div class="sidebar-head">
    <div class="section-label">Resources</div>
  </div>
  {{if .Sources}}
  <ul class="tree-list">
    {{range .Sources}}
    <li>
      <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>
        <div>{{.Label}}</div>
        <div class="tree-meta"><code>{{.Meta}}</code></div>
      </a>
    </li>
    {{end}}
  </ul>
  {{if .SelectedSourceLabel}}
  <div class="sidebar-preview stack">
    <h3>{{.SelectedSourceLabel}}</h3>
    <div class="meta"><code>{{.SelectedSourceMeta}}</code></div>
    <pre class="viewer">{{.SelectedSourceBody}}</pre>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No referenced source documents.</p>
  {{end}}
</section>`
