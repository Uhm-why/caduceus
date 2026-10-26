package compactor

import "regexp"

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)

func StripANSI(input string) string {
	return ansiPattern.ReplaceAllString(input, "")
}
