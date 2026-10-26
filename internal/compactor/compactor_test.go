package compactor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no ANSI",
			input:    "plain terminal text",
			expected: "plain terminal text",
		},
		{
			name:     "standard color codes",
			input:    "\x1b[32mPASS\x1b[0m: all tests passed",
			expected: "PASS: all tests passed",
		},
		{
			name:     "bold and 256 color",
			input:    "\x1b[1;38;5;196mError:\x1b[0m compilation failed",
			expected: "Error: compilation failed",
		},
		{
			name:     "cursor and line clearing",
			input:    "building\x1b[2K\rbuilt successfully",
			expected: "building\rbuilt successfully",
		},
		{
			name:     "OSC title sequence",
			input:    "\x1b]0;Window Title\x07Clean output text",
			expected: "Clean output text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestCollapseRepeatedLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single line",
			input:    "starting server",
			expected: "starting server",
		},
		{
			name:     "no duplicates",
			input:    "line 1\nline 2\nline 3",
			expected: "line 1\nline 2\nline 3",
		},
		{
			name:     "consecutive duplicates at start and end",
			input:    "ping\nping\nping\npong\npong",
			expected: "ping [repeated 3 times]\npong [repeated 2 times]",
		},
		{
			name:     "duplicates in middle",
			input:    "start\ndownloading...\ndownloading...\ndownloading...\nfinish",
			expected: "start\ndownloading... [repeated 3 times]\nfinish",
		},
		{
			name:     "all identical lines",
			input:    "tick\ntick\ntick\ntick",
			expected: "tick [repeated 4 times]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollapseRepeatedLines(tt.input)
			if got != tt.expected {
				t.Fatalf("expected:\n%s\n\ngot:\n%s", tt.expected, got)
			}
		})
	}
}

func TestWindowAndSpill(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		MaxLines:     20,
		HeadLines:    5,
		TailLines:    5,
		SpilloverDir: tmpDir,
		EnableSpill:  true,
	}

	var rawLines []string
	for i := 0; i < 30; i++ {
		rawLines = append(rawLines, fmt.Sprintf("line %02d", i))
	}
	input := strings.Join(rawLines, "\n")

	out, err := WindowAndSpill(input, "task-spill-001", cfg)
	if err != nil {
		t.Fatalf("unexpected error from WindowAndSpill: %v", err)
	}

	outLines := strings.Split(out, "\n")
	if outLines[0] != "line 00" || outLines[4] != "line 04" {
		t.Fatalf("head lines mismatch, got: %v", outLines[:5])
	}

	lastLine := outLines[len(outLines)-1]
	if lastLine != "line 29" {
		t.Fatalf("expected last line to be 'line 29', got %q", lastLine)
	}

	if !strings.Contains(out, "20 lines omitted. Full output saved to:") {
		t.Fatalf("expected marker with 20 omitted lines, got:\n%s", out)
	}

	spillPath := filepath.Join(tmpDir, "task-spill-001.txt")
	diskBytes, err := os.ReadFile(spillPath)
	if err != nil {
		t.Fatalf("failed to read spilled file from disk: %v", err)
	}
	if string(diskBytes) != input {
		t.Fatalf("file content on disk did not match untruncated input")
	}

	t.Run("graceful fallback when head plus tail exceeds total lines", func(t *testing.T) {
		smallInput := "line 1\nline 2\nline 3"
		cfgFallback := Config{
			MaxLines:  2,
			HeadLines: 2,
			TailLines: 2,
		}
		res, err := WindowAndSpill(smallInput, "fallback", cfgFallback)
		if err != nil {
			t.Fatalf("unexpected error on fallback: %v", err)
		}
		if res != smallInput {
			t.Fatalf("expected untouched small input, got %q", res)
		}
	})

	t.Run("marker when EnableSpill is false", func(t *testing.T) {
		noSpillCfg := Config{
			MaxLines:    5,
			HeadLines:   2,
			TailLines:   2,
			EnableSpill: false,
		}
		tenLines := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10"
		noSpillOut, err := WindowAndSpill(tenLines, "no-spill", noSpillCfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(noSpillOut, "Full output saved to:") {
			t.Fatalf("expected no disk path mentioned when EnableSpill=false, got:\n%s", noSpillOut)
		}
		if !strings.Contains(noSpillOut, "[... 6 lines omitted ...]") {
			t.Fatalf("expected omitted count marker, got:\n%s", noSpillOut)
		}
	})
}

func TestCompactor_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		MaxLines:     10,
		HeadLines:    3,
		TailLines:    3,
		SpilloverDir: tmpDir,
		EnableSpill:  true,
	}

	c := New(cfg)

	t.Run("empty input returns immediately", func(t *testing.T) {
		res, err := c.Compact("", "empty-task")
		if err != nil {
			t.Fatalf("unexpected error on empty: %v", err)
		}
		if res != "" {
			t.Fatalf("expected empty string, got %q", res)
		}
	})

	t.Run("full 3 stage pipeline execution", func(t *testing.T) {
		var noisy []string
		for i := 0; i < 8; i++ {
			noisy = append(noisy, "\x1b[32m[PASS] running chunk...\x1b[0m")
		}
		for i := 0; i < 20; i++ {
			noisy = append(noisy, fmt.Sprintf("\x1b[33m[WARN] item %d check\x1b[0m", i))
		}
		for i := 0; i < 6; i++ {
			noisy = append(noisy, "\x1b[31m[FAIL] connection timeout\x1b[0m")
		}

		payload := strings.Join(noisy, "\n")

		compacted, err := c.Compact(payload, "pipeline-test")
		if err != nil {
			t.Fatalf("unexpected error during Compact: %v", err)
		}

		if strings.Contains(compacted, "\x1b[") {
			t.Fatalf("found unstripped ANSI escape sequences in compacted output")
		}

		if !strings.Contains(compacted, "[PASS] running chunk... [repeated 8 times]") {
			t.Fatalf("expected repeated chunk to be collapsed, got:\n%s", compacted)
		}
		if !strings.Contains(compacted, "[FAIL] connection timeout [repeated 6 times]") {
			t.Fatalf("expected repeated fail to be collapsed, got:\n%s", compacted)
		}

		if !strings.Contains(compacted, "Full output saved to:") {
			t.Fatalf("expected spillover marker since line threshold was exceeded, got:\n%s", compacted)
		}

		spillFile := filepath.Join(tmpDir, "pipeline-test.txt")
		if _, err := os.Stat(spillFile); err != nil {
			t.Fatalf("spillover file was not created on disk: %v", err)
		}
	})
}
