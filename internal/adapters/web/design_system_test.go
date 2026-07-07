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
