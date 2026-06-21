package runtimes

import (
	"regexp"
	"strings"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var versionLineRE = regexp.MustCompile(`(?i)(picoclaw|crush)[^\n\r]*?(v?\d+\.\d+\.\d+)`)
var semverRE = regexp.MustCompile(`v?\d+\.\d+\.\d+[^\s\n\r]*`)

func sanitizeTerminalOutput(raw string) string {
	clean := ansiEscapeRE.ReplaceAllString(raw, "")
	clean = strings.ReplaceAll(clean, "\r", "\n")
	lines := strings.Split(clean, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func extractRuntimeVersion(runtimeID string, output string) string {
	clean := sanitizeTerminalOutput(output)
	if match := versionLineRE.FindStringSubmatch(clean); len(match) >= 3 {
		return strings.TrimSpace(match[1] + " " + match[2])
	}
	if match := semverRE.FindString(clean); match != "" {
		if runtimeID != "" {
			return runtimeID + " " + strings.TrimSpace(match)
		}
		return strings.TrimSpace(match)
	}
	return clean
}
