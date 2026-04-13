package taskbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultStorePathUsesTaskbenchDataDir(t *testing.T) {
	customDir := filepath.Join(t.TempDir(), "taskbench-data")
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

	options, err := parseRunOptions([]string{"taskbench", "--data-dir", customDir, "--seed-demo"})
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
	if _, err := parseRunOptions([]string{"taskbench", "extra"}); err == nil {
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

	options, err := parseRunOptions([]string{"taskbench"})
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

	if code := runConfigSet([]string{"taskbench", "config", "set", "--data-dir", dataDir}); code != 0 {
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
