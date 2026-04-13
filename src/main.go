package taskbench

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(args []string) int {
	if isConfigCommand(args) {
		return runConfigCommand(args)
	}
	if isVaultCommand(args) {
		return runVaultCommand(args)
	}

	options, err := parseRunOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	store := NewStore(options.storePath)
	if options.seedDemo {
		if err := store.Save(demoState(time.Now())); err != nil {
			fmt.Fprintf(os.Stderr, "seed demo data: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "demo data written to %s\n", store.vault.RootDir())
		return 0
	}

	state, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load vault: %v\n", err)
		return 1
	}
	app := NewApp(store, state)
	themes, err := store.vault.LoadThemes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load themes: %v\n", err)
		return 1
	}
	app.themes = themes
	app.loadState = store.Load
	app.saveState = store.Save
	app.view = viewWorkbench
	app.selectedSection = sectionIssueNoStatus
	app.actionSection = sectionToday
	app.focus = paneSidebar
	app.resolveRef = func(ref string) (string, error) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return "", fmt.Errorf("empty ref")
		}
		if filepath.IsAbs(ref) {
			return ref, nil
		}
		return filepath.Join(store.vault.RootDir(), ref), nil
	}
	app.issueAssetSummary = func(id string) IssueAssetSummary {
		summary, err := store.vault.SummarizeIssue(id)
		if err != nil {
			return IssueAssetSummary{}
		}
		return summary
	}
	app.themeAssetSummary = func(id string) ThemeAssetSummary {
		summary, err := store.vault.SummarizeTheme(id)
		if err != nil {
			return ThemeAssetSummary{}
		}
		return summary
	}
	app.status = "Inbox, tasks, issues, and themes are backed by vault/."

	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run app: %v\n", err)
		return 1
	}
	return 0
}

type runOptions struct {
	storePath string
	seedDemo  bool
}

func parseRunOptions(args []string) (runOptions, error) {
	defaultPath, err := defaultStorePath()
	if err != nil {
		return runOptions{}, fmt.Errorf("resolve store path: %w", err)
	}

	options := runOptions{}
	fs := flag.NewFlagSet(flagSetName(args), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&options.storePath, "data-dir", defaultPath, "directory used to store taskbench data")
	fs.BoolVar(&options.seedDemo, "seed-demo", false, "write demo data to the active store")

	if err := fs.Parse(args[1:]); err != nil {
		return runOptions{}, fmt.Errorf("parse args: %w", err)
	}
	if fs.NArg() > 0 {
		return runOptions{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	options.storePath, err = filepath.Abs(options.storePath)
	if err != nil {
		return runOptions{}, fmt.Errorf("resolve store path: %w", err)
	}
	return options, nil
}

func flagSetName(args []string) string {
	if len(args) == 0 {
		return "taskbench"
	}
	return args[0]
}

func isConfigCommand(args []string) bool {
	return len(args) > 1 && args[1] == "config"
}

func runConfigCommand(args []string) int {
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s config <show|path|set>\n", flagSetName(args))
		return 1
	}

	switch args[2] {
	case "show":
		return runConfigShow()
	case "path":
		return runConfigPath()
	case "set":
		return runConfigSet(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown config command: %s\n", args[2])
		return 1
	}
}

func runConfigShow() int {
	configPath, err := defaultConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve config path: %v\n", err)
		return 1
	}
	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}

	payload := struct {
		ConfigPath string `json:"config_path"`
		DataDir    string `json:"data_dir,omitempty"`
	}{
		ConfigPath: configPath,
		DataDir:    config.DataDir,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "render config: %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, string(raw))
	return 0
}

func runConfigPath() int {
	configPath, err := defaultConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve config path: %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, configPath)
	return 0
}

func runConfigSet(args []string) int {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", "", "directory used to store taskbench data")

	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", fs.Args())
		return 1
	}
	if strings.TrimSpace(*dataDir) == "" {
		fmt.Fprintln(os.Stderr, "config set requires --data-dir")
		return 1
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve config path: %v\n", err)
		return 1
	}
	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	config.DataDir = *dataDir
	if err := saveConfig(configPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "config updated: %s\n", configPath)
	return 0
}
