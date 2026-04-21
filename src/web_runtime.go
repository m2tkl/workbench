package workbench

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
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
	fmt.Fprintf(os.Stdout, "web ui listening on %s\n", baseURL)
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
	workbenchTmpl      *template.Template
	vault              VaultFS
	sourceTmpl         *template.Template
	eventsTmpl         *template.Template
	eventCreateTmpl    *template.Template
	workspaceTmpl      *template.Template
	eventWorkspaceTmpl *template.Template
	agentPaneTmpl      *template.Template
}

func newSourceWorkbenchServer(vault VaultFS) *sourceWorkbenchServer {
	return &sourceWorkbenchServer{
		workbenchTmpl:      template.Must(template.New("web-workbench").Parse(workbenchHTML)),
		vault:              vault,
		sourceTmpl:         template.Must(template.New("source-workbench").Parse(sourceWorkbenchHTML)),
		eventsTmpl:         template.Must(template.New("event-workbench").Parse(eventWorkbenchHTML)),
		eventCreateTmpl:    template.Must(template.New("event-create").Parse(eventCreateHTML)),
		workspaceTmpl:      template.Must(template.New("work-item-workspace").Parse(workItemWorkspaceHTML)),
		eventWorkspaceTmpl: template.Must(template.New("event-workspace").Parse(eventWorkspaceHTML)),
		agentPaneTmpl:      template.Must(template.New("work-item-agent-pane").Parse(workItemAgentPaneHTML)),
	}
}

func (s *sourceWorkbenchServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWorkbenchIndex)
	mux.HandleFunc("/sources", s.handleSourceIndex)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/paste", s.handlePaste)
	mux.HandleFunc("/link", s.handleLink)
	mux.HandleFunc("/workbench/add", s.handleWorkbenchAdd)
	mux.HandleFunc("/workbench/themes/create", s.handleWorkbenchThemeCreate)
	mux.HandleFunc("/workbench/items/", s.handleWorkbenchItemAction)
	mux.HandleFunc("/events", s.handleEventsIndex)
	mux.HandleFunc("/events/new", s.handleEventNew)
	mux.HandleFunc("/events/create", s.handleEventCreate)
	mux.HandleFunc("/events/", s.handleEvent)
	mux.HandleFunc("/work-items/", s.handleWorkItem)
	return mux
}
