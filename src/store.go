package taskbench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	root  string
	vault VaultFS
}

func defaultStorePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("TASKBENCH_DATA_DIR")); path != "" {
		return filepath.Abs(path)
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		return "", err
	}
	config, err := loadConfig(configPath)
	if err != nil {
		return "", err
	}
	if config.DataDir != "" {
		return config.DataDir, nil
	}
	return os.Getwd()
}

func NewStore(root string) Store {
	return Store{root: root, vault: NewVault(root)}
}

func (s Store) RootDir() string {
	return s.root
}

func (s Store) Load() (State, error) {
	return LoadVaultState(s.vault)
}

func (s Store) Save(state State) error {
	return SaveVaultState(s.vault, state)
}

func (s Store) EnsureNoteFile(item Item) (string, error) {
	if err := s.vault.EnsureLayout(); err != nil {
		return "", err
	}

	switch normalizeEntityForSave(item) {
	case entityInbox:
		if err := s.vault.SaveInboxItem(inboxFromItem(item)); err != nil {
			return "", err
		}
		return s.vault.InboxPath(item.ID), nil
	case entityIssue:
		if err := s.vault.SaveIssue(issueFromItem(item)); err != nil {
			return "", err
		}
		return s.vault.IssueMetaPath(item.ID), nil
	case entityTask:
		if err := s.vault.SaveTask(taskFromItem(item)); err != nil {
			return "", err
		}
		return s.vault.TaskMetaPath(item.ID), nil
	default:
		return "", fmt.Errorf("unknown entity type: %s", item.EntityType)
	}
}
