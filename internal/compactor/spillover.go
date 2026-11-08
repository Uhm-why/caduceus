package compactor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WindowAndSpill truncates text exceeding cfg.MaxLines into a head-and-tail window
// separated by an omission marker. If cfg.EnableSpill is true, the full text is
// saved to a file in cfg.SpilloverDir named by id.
func WindowAndSpill(text string, id string, cfg Config) (string, error) {
	lines := strings.Split(text, "\n")

	if len(lines) <= cfg.MaxLines {
		return text, nil
	}

	var spillDir string
	if cfg.SpilloverDir == "" {
		home, _ := os.UserHomeDir()
		spillDir = filepath.Join(home, ".config", "caduceus", "spillover")
	} else {
		spillDir = cfg.SpilloverDir
	}

	safeID := filepath.Base(id)
	filePath := filepath.Join(spillDir, safeID+".txt")

	if cfg.EnableSpill {
		if err := os.MkdirAll(spillDir, 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filePath, []byte(text), 0644); err != nil {
			return "", err
		}
	}

	if cfg.HeadLines+cfg.TailLines >= len(lines) {
		return text, nil
	}

	head := lines[:cfg.HeadLines]
	tail := lines[len(lines)-cfg.TailLines:]
	omittedCount := len(lines) - cfg.HeadLines - cfg.TailLines

	var marker string
	if cfg.EnableSpill {
		marker = fmt.Sprintf("\n[... %d lines omitted. Full output saved to: %s ...]\n", omittedCount, filePath)
	} else {
		marker = fmt.Sprintf("\n[... %d lines omitted ...]\n", omittedCount)
	}
	return strings.Join(head, "\n") + marker + strings.Join(tail, "\n"), nil

}
