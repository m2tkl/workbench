package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultStorePathUsesWorkbenchDataDir(t *testing.T) {
	customDir := filepath.Join(t.TempDir(), "workbench-data")
	t.Setenv("TASKBENCH_DATA_DIR", customDir)

	got, err := defaultStorePath()
	if err != nil {
		t.Fatalf("defaultStorePath returned error: %v", err)
	}

	want, err := filepath.Abs(customDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("defaultStorePath = %q, want %q", got, want)
	}
}

func TestDefaultStorePathUsesConfigDataDir(t *testing.T) {
	configDir := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "repo-data")
	t.Setenv("TASKBENCH_DATA_DIR", "")
	t.Setenv("TASKBENCH_CONFIG_DIR", configDir)

	configPath, err := defaultConfigPath()
	if err != nil {
		t.Fatalf("defaultConfigPath returned error: %v", err)
	}
	if err := saveConfig(configPath, Config{DataDir: dataDir}); err != nil {
		t.Fatalf("saveConfig returned error: %v", err)
	}

	got, err := defaultStorePath()
	if err != nil {
		t.Fatalf("defaultStorePath returned error: %v", err)
	}

	want, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("defaultStorePath = %q, want %q", got, want)
	}
}

func TestParseRunOptionsPrefersExplicitDataDir(t *testing.T) {
	t.Setenv("TASKBENCH_DATA_DIR", filepath.Join(t.TempDir(), "from-env"))
	customDir := filepath.Join(t.TempDir(), "custom")

	options, err := parseRunOptions([]string{"workbench", "ui", "--data-dir", customDir, "--seed-demo"}, 2)
	if err != nil {
		t.Fatalf("parseRunOptions returned error: %v", err)
	}

	want, err := filepath.Abs(customDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if options.storePath != want {
		t.Fatalf("storePath = %q, want %q", options.storePath, want)
	}
	if !options.seedDemo {
		t.Fatal("expected seedDemo to be true")
	}
}

func TestParseRunOptionsRejectsUnexpectedArgs(t *testing.T) {
	if _, err := parseRunOptions([]string{"workbench", "ui", "extra"}, 2); err == nil {
		t.Fatal("expected parseRunOptions to reject unexpected arguments")
	}
}

func TestParseRunOptionsDefaultsToWorkingDirectoryWithoutEnvOrConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TASKBENCH_DATA_DIR", "")
	t.Setenv("TASKBENCH_CONFIG_DIR", configDir)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}

	options, err := parseRunOptions([]string{"workbench", "ui"}, 2)
	if err != nil {
		t.Fatalf("parseRunOptions returned error: %v", err)
	}
	if options.storePath != wd {
		t.Fatalf("storePath = %q, want %q", options.storePath, wd)
	}
}

func TestRunConfigSetWritesConfig(t *testing.T) {
	configDir := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "repo-data")
	t.Setenv("TASKBENCH_CONFIG_DIR", configDir)

	if code := runConfigSet([]string{"workbench", "config", "set", "--data-dir", dataDir}); code != 0 {
		t.Fatalf("runConfigSet exit code = %d, want 0", code)
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		t.Fatalf("defaultConfigPath returned error: %v", err)
	}
	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	want, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if config.DataDir != want {
		t.Fatalf("data dir = %q, want %q", config.DataDir, want)
	}
}

func TestRunConfigEditUsesEditor(t *testing.T) {
	configDir := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "edited-data")
	t.Setenv("TASKBENCH_CONFIG_DIR", configDir)

	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n{\n  \"data_dir\": \"" + dataDir + "\"\n}\nEOF\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	t.Setenv("EDITOR", editorPath)

	if code := runConfigEdit([]string{"workbench", "config", "edit"}); code != 0 {
		t.Fatalf("runConfigEdit exit code = %d, want 0", code)
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		t.Fatalf("defaultConfigPath returned error: %v", err)
	}
	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	want, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if config.DataDir != want {
		t.Fatalf("data dir = %q, want %q", config.DataDir, want)
	}
}

func TestRunConfigEditRequiresEditor(t *testing.T) {
	t.Setenv("EDITOR", "")
	if code := runConfigEdit([]string{"workbench", "config", "edit"}); code == 0 {
		t.Fatal("expected runConfigEdit to fail without $EDITOR")
	}
}

func TestRunRootHelpIncludesTopLevelCommands(t *testing.T) {
	output := captureStdout(t, func() {
		if code := Run([]string{"workbench", "--help"}); code != 0 {
			t.Fatalf("Run exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"Usage:",
		"workbench ui [--data-dir DIR] [--seed-demo]",
		"Commands:",
		"ui",
		"vault",
		"config",
		"web",
		"Examples:",
		"workbench vault --help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(strings.ToLower(output), "nix") {
		t.Fatalf("root help unexpectedly mentions nix:\n%s", output)
	}
}

func TestRunRootHelpNormalizesExecutableName(t *testing.T) {
	output := captureStdout(t, func() {
		if code := Run([]string{"/tmp/nix-shell.qWit5o/go-build1164112336/b001/exe/workbench", "--help"}); code != 0 {
			t.Fatalf("Run exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"workbench ui [--data-dir DIR] [--seed-demo]",
		"workbench vault --help",
		"workbench config show",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "/tmp/nix-shell.") {
		t.Fatalf("help leaked temp executable path:\n%s", output)
	}
}

func TestRunWithoutArgsShowsHelp(t *testing.T) {
	output := captureStdout(t, func() {
		if code := Run([]string{"workbench"}); code != 0 {
			t.Fatalf("Run exit code = %d, want 0", code)
		}
	})

	if !strings.Contains(output, "workbench ui [--data-dir DIR] [--seed-demo]") {
		t.Fatalf("help output missing ui usage:\n%s", output)
	}
}

func TestRunUISeedDemoWritesDemoData(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo-data")
	output := captureStdout(t, func() {
		if code := Run([]string{"workbench", "ui", "--data-dir", root, "--seed-demo"}); code != 0 {
			t.Fatalf("Run exit code = %d, want 0", code)
		}
	})

	if !strings.Contains(output, "demo data written to") {
		t.Fatalf("expected seed-demo output, got:\n%s", output)
	}
}

func TestRunRejectsTopLevelFlagsWithoutUICommand(t *testing.T) {
	if code := Run([]string{"workbench", "--seed-demo"}); code == 0 {
		t.Fatal("expected top-level --seed-demo without ui to fail")
	}
}
