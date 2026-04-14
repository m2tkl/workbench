package workbench

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultSourceWorkbenchAddr = "127.0.0.1:8080"

func isWebCommand(args []string) bool {
	return len(args) > 1 && args[1] == "web"
}

func runWebCommand(args []string) int {
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s web serve [--data-dir DIR] [--addr ADDR]\n", flagSetName(args))
		return 1
	}
	switch args[2] {
	case "serve":
		return runWebServe(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown web command: %s\n", args[2])
		return 1
	}
}

func runWebServe(args []string) int {
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("web serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	addr := fs.String("addr", defaultSourceWorkbenchAddr, "HTTP listen address")
	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	root, err := filepath.Abs(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		return 1
	}
	store := NewStore(root)
	if err := store.vault.EnsureLayout(); err != nil {
		fmt.Fprintf(os.Stderr, "init vault: %v\n", err)
		return 1
	}
	runtime := newSourceWorkbenchRuntime(store.vault, *addr)
	baseURL, err := runtime.EnsureStarted()
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve web ui: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "web source inbox listening on %s\n", baseURL)
	if err := runtime.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "serve web ui: %v\n", err)
		return 1
	}
	return 0
}

type sourceWorkbenchRuntime struct {
	vault   VaultFS
	addr    string
	baseURL string

	mu      sync.Mutex
	started bool
	server  *http.Server
	errCh   chan error
}

func newSourceWorkbenchRuntime(vault VaultFS, addr string) *sourceWorkbenchRuntime {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = defaultSourceWorkbenchAddr
	}
	return &sourceWorkbenchRuntime{
		vault: vault,
		addr:  addr,
	}
}

func (r *sourceWorkbenchRuntime) EnsureStarted() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return r.baseURL, nil
	}
	listener, err := net.Listen("tcp", r.addr)
	if err != nil {
		return "", err
	}
	r.baseURL = "http://" + listener.Addr().String()
	r.server = &http.Server{Handler: newSourceWorkbenchServer(r.vault).routes()}
	r.errCh = make(chan error, 1)
	r.started = true

	go func() {
		err := r.server.Serve(listener)
		if err == nil || err == http.ErrServerClosed {
			r.errCh <- nil
			return
		}
		r.errCh <- err
	}()

	return r.baseURL, nil
}

func (r *sourceWorkbenchRuntime) Wait() error {
	r.mu.Lock()
	errCh := r.errCh
	r.mu.Unlock()
	if errCh == nil {
		return nil
	}
	return <-errCh
}

func (r *sourceWorkbenchRuntime) Stop() error {
	r.mu.Lock()
	server := r.server
	started := r.started
	r.server = nil
	r.baseURL = ""
	r.started = false
	r.errCh = nil
	r.mu.Unlock()

	if !started || server == nil {
		return nil
	}
	return server.Shutdown(context.Background())
}

func buildSourceWorkbenchURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://" + defaultSourceWorkbenchAddr
	}
	return baseURL + "/"
}

type sourceWorkbenchServer struct {
	vault VaultFS
	tmpl  *template.Template
}

type sourceWorkbenchPage struct {
	StagedFiles []string
	Status      string
	Error       string
}

func newSourceWorkbenchServer(vault VaultFS) *sourceWorkbenchServer {
	return &sourceWorkbenchServer{
		vault: vault,
		tmpl:  template.Must(template.New("source-workbench").Parse(sourceWorkbenchHTML)),
	}
}

func (s *sourceWorkbenchServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/upload", s.handleUpload)
	return mux
}

func (s *sourceWorkbenchServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	stagedFiles, err := s.vault.ListStagedSourceFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := sourceWorkbenchPage{
		StagedFiles: stagedFiles,
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		Error:       strings.TrimSpace(r.URL.Query().Get("error")),
	}
	if err := s.tmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		s.redirectWithMessage(w, r, "", fmt.Sprintf("upload form parse failed: %v", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.redirectWithMessage(w, r, "", "file is required")
		return
	}
	defer file.Close()
	stagedName, err := s.vault.StageSourceUpload(header.Filename, file)
	if err != nil {
		s.redirectWithMessage(w, r, "", err.Error())
		return
	}
	s.redirectWithMessage(w, r, fmt.Sprintf("staged %s", stagedName), "")
}

func (s *sourceWorkbenchServer) redirectWithMessage(w http.ResponseWriter, r *http.Request, status, errMsg string) {
	values := url.Values{}
	if status != "" {
		values.Set("status", status)
	}
	if errMsg != "" {
		values.Set("error", errMsg)
	}
	http.Redirect(w, r, "/?"+values.Encode(), http.StatusSeeOther)
}

const sourceWorkbenchHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Workbench Sources</title>
  <style>
    :root {
      --bg: #ffffff;
      --ink: #111111;
      --muted: #666666;
      --line: #dddddd;
      --accent: #111111;
      --error: #b00020;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--ink);
    }
    main {
      max-width: 720px;
      margin: 0 auto;
      padding: 24px 16px 48px;
    }
    h1 {
      margin: 0 0 6px;
      font-size: 1.4rem;
      font-weight: 600;
    }
    p.lead {
      margin: 0 0 20px;
      color: var(--muted);
      font-size: 0.95rem;
    }
    .section {
      padding: 0;
      margin-bottom: 24px;
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    label {
      display: block;
      font-size: 0.85rem;
      margin-bottom: 4px;
    }
    select, input[type="file"], button {
      width: 100%;
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 10px 12px;
      font: inherit;
      background: #fff;
      color: var(--ink);
    }
    button {
      cursor: pointer;
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    .notice {
      padding: 10px 12px;
      border-radius: 4px;
      margin-bottom: 12px;
      font-size: 0.92rem;
    }
    .notice.ok {
      background: #f6f6f6;
    }
    .notice.error {
      color: var(--error);
      background: #fff7f8;
    }
    ul.files {
      list-style: none;
      padding: 0;
      margin: 0;
    }
    ul.files li {
      display: flex;
      gap: 12px;
      align-items: flex-start;
      padding: 10px 0;
      border-top: 1px solid var(--line);
    }
    ul.files li:first-child {
      border-top: 0;
      padding-top: 0;
    }
    .meta {
      color: var(--muted);
      font-size: 0.86rem;
    }
    .empty {
      color: var(--muted);
    }
    h2 {
      margin: 0 0 10px;
      font-size: 1rem;
      font-weight: 600;
    }
    .actions {
      margin-top: 12px;
    }
    @media (max-width: 640px) {
      main { padding: 16px 12px 32px; }
    }
  </style>
</head>
<body>
  <main>
    <h1>Source Inbox</h1>
    <p class="lead">Upload files into <code>sources/files/staged/</code>. Classify and extract them later.</p>
    {{if .Status}}<div class="notice ok">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}

    <section class="section">
      <h2>Upload</h2>
      <form method="post" action="/upload" enctype="multipart/form-data">
        <div class="stack">
          <div>
            <label for="file">File</label>
            <input id="file" type="file" name="file" required>
          </div>
          <div>
            <button type="submit">Stage Upload</button>
          </div>
        </div>
        <p class="meta">Drop or pick a file. It will stay in <code>sources/files/staged/</code> until an agent or CLI flow processes it.</p>
      </form>
    </section>

    <section class="section">
      <h2>Staged Files</h2>
      {{if .StagedFiles}}
      <ul class="files">
        {{range .StagedFiles}}
        <li>
          <div>
            <div>{{.}}</div>
            <div class="meta">Staged in <code>sources/files/staged/</code>. Extract this later with an agent or CLI flow.</div>
          </div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="empty">No staged files yet.</p>
      {{end}}
    </section>
  </main>
</body>
</html>`
