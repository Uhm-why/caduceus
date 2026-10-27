package compactor

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (c *Compactor) Prune(maxAge time.Duration) (int, error) {
	spillDir := c.config.SpilloverDir
	if spillDir == "" {
		home, _ := os.UserHomeDir()
		spillDir = filepath.Join(home, ".config", "caduceus", "spillover")
	}

	if _, err := os.Stat(spillDir); os.IsNotExist(err) {
		return 0, nil
	}

	entries, err := os.ReadDir(spillDir)
	if err != nil {
		return 0, err
	}

	var prunedCount int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > maxAge {
			if err := os.Remove(filepath.Join(spillDir, entry.Name())); err == nil {
				prunedCount++
			}
		}
	}
	return prunedCount, nil
}

func (c *Compactor) DeleteSpillover(id string) error {
	spillDir := c.config.SpilloverDir
	if spillDir == "" {
		home, _ := os.UserHomeDir()
		spillDir = filepath.Join(home, ".config", "caduceus", "spillover")
	}

	safeID := filepath.Base(id)

	target := filepath.Join(spillDir, safeID+".txt")

	err := os.Remove(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
