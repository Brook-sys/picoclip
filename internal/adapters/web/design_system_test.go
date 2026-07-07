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

func TestOverviewCardsUseCanonicalHelperAndCSS(t *testing.T) {
	t.Parallel()

	uiBody, err := os.ReadFile("ui.templ")
	if err != nil {
		t.Fatalf("read ui.templ: %v", err)
	}
	ui := string(uiBody)
	if !strings.Contains(ui, "templ OverviewGrid()") || !strings.Contains(ui, "templ OverviewCard(") {
		t.Fatalf("ui.templ must define canonical OverviewGrid and OverviewCard helpers")
	}
	if !strings.Contains(ui, `class="pc-overview-grid"`) || !strings.Contains(ui, `"pc-overview-card"`) {
		t.Fatalf("overview helpers must emit canonical pc-overview-* classes")
	}

	for _, name := range []string{"dashboard.templ", "tasks_page.templ", "runs_page.templ", "activity.templ"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		if !strings.Contains(text, "@OverviewGrid()") || !strings.Contains(text, "@OverviewCard(") {
			t.Fatalf("%s must render overview stats through OverviewGrid and OverviewCard helpers", name)
		}
		for _, legacy := range []string{"dashboard-overview-card", "tasks-overview-card", "runs-overview-card", "activity-overview-card"} {
			if strings.Contains(text, legacy) {
				t.Fatalf("%s still contains legacy %s markup; use OverviewCard helper instead", name, legacy)
			}
		}
	}

	cssBody, err := os.ReadFile("assets/app.css")
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	css := string(cssBody)
	for _, selector := range []string{".pc-overview-grid", ".pc-overview-card", ".pc-overview-card.live", ".pc-overview-card.attention", ".pc-overview-card.success", ".pc-overview-card.error", ".pc-overview-card.muted"} {
		_ = cssBlock(t, css, selector)
	}
	for _, legacy := range []string{".dashboard-overview-card", ".tasks-overview-card", ".runs-overview-card", ".activity-overview-card"} {
		if strings.Contains(css, legacy) {
			t.Fatalf("app.css still contains legacy selector %s; use pc-overview-* selectors instead", legacy)
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
