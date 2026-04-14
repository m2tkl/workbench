package taskbench

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(args []string) int {
	switch commandArg(args, 1) {
	case "":
		printTopLevelHelp(args)
		return 0
	case "-h", "--help":
		printTopLevelHelp(args)
		return 0
	case "ui":
		return runUICommand(args)
	case "config":
		return runConfigCommand(args)
	case "vault":
		return runVaultCommand(args)
	case "web":
		return runWebCommand(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", strings.TrimSpace(args[1]))
		printTopLevelHelp(args)
		return 1
	}
}

func printTopLevelHelp(args []string) {
	printHelp(topLevelHelp(args))
}

func topLevelHelp(args []string) commandHelp {
	return commandHelp{
		Usage: []string{
			fmt.Sprintf("%s ui [--data-dir DIR] [--seed-demo]", flagSetName(args)),
			fmt.Sprintf("%s <vault|config|web> ...", flagSetName(args)),
		},
		Description: "Run the taskbench TUI or use subcommands to inspect and manage vault-backed data.",
		Commands: []helpCommand{
			{Name: "ui", Summary: "Launch the taskbench TUI."},
			{Name: "vault", Summary: "Manage inbox captures, tasks, issues, themes, and sources."},
			{Name: "config", Summary: "Show or update persisted CLI configuration."},
			{Name: "web", Summary: "Serve the source inbox web UI."},
		},
		Examples: []string{
			fmt.Sprintf("%s ui", flagSetName(args)),
			fmt.Sprintf("%s ui --seed-demo", flagSetName(args)),
			fmt.Sprintf("%s vault --help", flagSetName(args)),
			fmt.Sprintf("%s config show", flagSetName(args)),
		},
	}
}

func runUICommand(args []string) int {
	options, err := parseRunOptions(args, 2)
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
	workbenchRuntime := newSourceWorkbenchRuntime(store.vault, defaultSourceWorkbenchAddr)
	app.startSourceWorkbench = workbenchRuntime.EnsureStarted
	app.stopSourceWorkbench = workbenchRuntime.Stop
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

func parseRunOptions(args []string, start int) (runOptions, error) {
	defaultPath, err := defaultStorePath()
	if err != nil {
		return runOptions{}, fmt.Errorf("resolve store path: %w", err)
	}

	options := runOptions{}
	fs := flag.NewFlagSet(flagSetName(args), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&options.storePath, "data-dir", defaultPath, "directory used to store taskbench data")
	fs.BoolVar(&options.seedDemo, "seed-demo", false, "write demo data to the active store")

	if start < 0 {
		start = 0
	}
	if start > len(args) {
		start = len(args)
	}
	if err := fs.Parse(args[start:]); err != nil {
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
	name := strings.TrimSpace(args[0])
	if name == "" {
		return "taskbench"
	}
	base := path.Base(name)
	if base == "." || base == string(filepath.Separator) || strings.HasPrefix(base, "taskbench") {
		return "taskbench"
	}
	return base
}

func isConfigCommand(args []string) bool {
	return len(args) > 1 && args[1] == "config"
}

func isUICommand(args []string) bool {
	return len(args) > 1 && args[1] == "ui"
}

func runConfigCommand(args []string) int {
	if handled, exitCode := maybeHandleCommandHelp(args, 2, 3, configCommandHelp(args)); handled {
		return exitCode
	}

	switch args[2] {
	case "show":
		return runConfigShow()
	case "path":
		return runConfigPath()
	case "set":
		return runConfigSet(args)
	case "edit":
		return runConfigEdit(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown config command: %s\n", args[2])
		return 1
	}
}

func configCommandHelp(args []string) commandHelp {
	return commandHelp{
		Usage: []string{
			fmt.Sprintf("%s config <show|path|set|edit>", flagSetName(args)),
		},
		Description: "Inspect, update, or open the persisted taskbench config file.",
		Commands: []helpCommand{
			{Name: "show", Summary: "Print the config path and current values as JSON."},
			{Name: "path", Summary: "Print only the config file path."},
			{Name: "set", Summary: "Update config values from flags."},
			{Name: "edit", Summary: "Open the config file in $EDITOR."},
		},
		Examples: []string{
			fmt.Sprintf("%s config show", flagSetName(args)),
			fmt.Sprintf("%s config path", flagSetName(args)),
			fmt.Sprintf("%s config set --data-dir ./vault", flagSetName(args)),
			fmt.Sprintf("%s config edit", flagSetName(args)),
		},
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

func runConfigEdit(args []string) int {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		fmt.Fprintln(os.Stderr, "config edit requires $EDITOR")
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
	if err := saveConfig(configPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "initialize config: %v\n", err)
		return 1
	}

	editorArgs := strings.Fields(editor)
	if len(editorArgs) == 0 {
		fmt.Fprintln(os.Stderr, "config edit requires a valid $EDITOR")
		return 1
	}

	cmd := exec.Command(editorArgs[0], append(editorArgs[1:], configPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run editor: %v\n", err)
		return 1
	}
	return 0
}

func commandArg(args []string, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	return strings.TrimSpace(args[index])
}

func maybeHandleCommandHelp(args []string, helpIndex, minArgs int, help commandHelp) (bool, int) {
	if len(args) < minArgs {
		printHelp(help)
		return true, 1
	}
	if isHelpToken(commandArg(args, helpIndex)) {
		printHelp(help)
		return true, 0
	}
	return false, 0
}
