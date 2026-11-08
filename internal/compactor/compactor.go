// Package compactor implements deterministic context reduction for agent outputs,
// including ANSI escape sequence stripping, consecutive duplicate line deduplication,
// and head-and-tail windowing with disk spillover.
package compactor

import "strings"

// Config defines thresholds and behavior for context compaction.
type Config struct {
	MaxLines     int    // MaxLines is the maximum number of lines allowed before windowing and spillover occur.
	HeadLines    int    // HeadLines is the number of leading lines preserved during windowing.
	TailLines    int    // TailLines is the number of trailing lines preserved during windowing.
	SpilloverDir string // SpilloverDir is the directory where oversized payloads are written to disk.
	EnableSpill  bool   // EnableSpill specifies whether full output is persisted to disk when windowing.
}

// DefaultConfig returns the standard compaction settings:
// 100 max lines, 25 head lines, 50 tail lines, and spillover enabled.
func DefaultConfig() Config {
	return Config{
		MaxLines:    100,
		HeadLines:   25,
		TailLines:   50,
		EnableSpill: true,
	}
}

// Compactor executes deterministic context reduction pipelines using a Config.
type Compactor struct {
	config Config
}

// New constructs a Compactor with the specified Config.
func New(cfg Config) *Compactor {
	return &Compactor{
		config: cfg,
	}
}

// Compact processes input by stripping ANSI sequences, deduplicating repeated
// lines, and applying head-and-tail windowing with disk spillover if line count
// exceeds MaxLines.
func (c *Compactor) Compact(input string, id string) (string, error) {
	// If input is empty, return an empty string.
	if input == "" {
		return "", nil
	}

	cleaned := StripANSI(input) // Strip ANSI from input.

	deduped := CollapseRepeatedLines(cleaned) // Deduplicate repeated text from input.

	lines := strings.Split(deduped, "\n") // Split deduped text on new lines.

	// If the length of the cleaned and deduped lines is greater than MaxLines and EnableSpill is true,
	// spillover to the configured directory.
	if len(lines) > c.config.MaxLines {
		return WindowAndSpill(deduped, id, c.config)
	}

	return deduped, nil
}
