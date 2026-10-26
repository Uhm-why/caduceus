package compactor

import (
	"fmt"
	"strings"
)

func CollapseRepeatedLines(input string) string {
	if input == "" {
		return ""
	}

	lines := strings.Split(input, "\n")

	var result []string
	currentLine := lines[0]
	count := 1

	for i := 1; i < len(lines); i++ {
		if lines[i] == currentLine {
			count++
		} else {
			if count > 1 {
				result = append(result, fmt.Sprintf("%s [repeated %d times]", currentLine, count))
			} else {
				result = append(result, currentLine)
			}
			currentLine = lines[i]
			count = 1
		}
	}

	if count > 1 {
		result = append(result, fmt.Sprintf("%s [repeated %d times]", currentLine, count))
	} else {
		result = append(result, currentLine)
	}

	return strings.Join(result, "\n")
}
