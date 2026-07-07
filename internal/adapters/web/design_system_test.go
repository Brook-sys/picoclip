package web

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalActionAndBadgeHelpersReplaceLegacyProjectAndSkillMarkup(t *testing.T) {
	t.Parallel()

	files := []string{
		"project_detail.templ",
		"skill_detail.templ",
		"dashboard.templ",
		"skills.templ",
		"modals.templ",
		"layout.templ",
	}
	legacySnippets := []string{
		`class="button`,
		`class="badge`,
	}

	for _, name := range files {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		for _, snippet := range legacySnippets {
			if strings.Contains(text, snippet) {
				t.Fatalf("%s still contains legacy %s markup; use Button/ButtonLink/Badge helpers instead", name, snippet)
			}
		}
	}
}

func TestDesignSystemCSSDefinesDarkModeDepthAndContrastTokens(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("assets/app.css")
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	css := string(body)

	darkBlock := cssBlock(t, css, `[data-theme="dark"]`)
	requiredDarkTokens := []string{
		`--surface-overlay:`,
		`--surface-raised:`,
		`--text-strong:`,
		`--text-muted:`,
		`--shadow-glow:`,
	}
	for _, token := range requiredDarkTokens {
		if !strings.Contains(darkBlock, token) {
			t.Fatalf("dark theme block must define %s for accessible depth/contrast", token)
		}
	}

	for selector, want := range map[string]string{
		`.sidebar`:          `box-shadow: var(--shadow-glow);`,
		`.page-title-icon`:  `background: var(--surface-raised);`,
		`.pc-card`:          `background: var(--surface-raised);`,
		`.pc-input`:         `background: var(--surface-overlay);`,
		`.pc-btn-secondary`: `background: var(--surface-raised);`,
		`.run-log-label`:    `color: var(--text-muted);`,
		`.run-log-viewer`:   `box-shadow: inset 0 1px 0 var(--border);`,
	} {
		block := cssBlock(t, css, selector)
		if !strings.Contains(block, want) {
			t.Fatalf("%s must include %q for dark-mode contrast/depth; block was %q", selector, want, block)
		}
	}
}

func cssBlock(t *testing.T, css, selector string) string {
	t.Helper()
	start := strings.Index(css, selector+" {")
	if start == -1 {
		t.Fatalf("missing CSS selector %s", selector)
	}
	open := strings.Index(css[start:], "{")
	if open == -1 {
		t.Fatalf("missing opening brace for %s", selector)
	}
	blockStart := start + open + 1
	end := strings.Index(css[blockStart:], "}")
	if end == -1 {
		t.Fatalf("missing closing brace for %s", selector)
	}
	return css[blockStart : blockStart+end]
}
