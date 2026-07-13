package services

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestSkillServiceImportRemoteYAML(t *testing.T) {
	withRemoteSkillHTTPClient(t, http.StatusOK, `name: Release Workflow
description: Deploy changes safely.
instructions: |
  Verify tests before deployment.
files:
  - path: references/checklist.md
    content: Confirm rollback steps.
version: 1.2.0
`)

	storage := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)}
	service := NewSkillService(storage, clock, &TimeIDGenerator{})

	const sourceURL = "https://skills.example.test/release.yaml"
	skill, err := service.ImportRemoteYAML(context.Background(), "project_1", sourceURL)
	if err != nil {
		t.Fatalf("ImportRemoteYAML returned error: %v", err)
	}
	if skill.Name != "Release Workflow" || skill.Slug != "release-workflow" {
		t.Fatalf("imported skill identity = %#v", skill)
	}
	if skill.Source != sourceURL || skill.Version != "1.2.0" {
		t.Fatalf("imported skill source/version = %q/%q", skill.Source, skill.Version)
	}
	if len(skill.Files) != 1 || skill.Files[0].Path != "references/checklist.md" {
		t.Fatalf("imported files = %#v", skill.Files)
	}

	prompt, err := NewPromptBuilder(storage).Build(context.Background(), PromptBuildInput{Agent: agentWithSkills("agent_1", skill.ID), Task: testPromptTask("task_1"), Run: testPromptRun("run_1")})
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if !strings.Contains(prompt, "Verify tests before deployment.") || !strings.Contains(prompt, "Confirm rollback steps.") {
		t.Fatalf("prompt did not contain imported skill: %s", prompt)
	}
}

func TestSkillServiceImportRemoteYAMLRejectsInvalidDocument(t *testing.T) {
	withRemoteSkillHTTPClient(t, http.StatusOK, "name: Missing instructions\n")

	service := NewSkillService(memory.NewStorage(), fixedClock{t: time.Now().UTC()}, &TimeIDGenerator{})
	_, err := service.ImportRemoteYAML(context.Background(), "", "https://skills.example.test/missing.yaml")
	if err == nil || !strings.Contains(err.Error(), "name and instructions are required") {
		t.Fatalf("ImportRemoteYAML error = %v, want invalid skill document", err)
	}
}

func TestSkillServiceImportRemoteYAMLRejectsPrivateAddress(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("private remote address must not be fetched")
	}))
	defer remote.Close()

	service := NewSkillService(memory.NewStorage(), fixedClock{t: time.Now().UTC()}, &TimeIDGenerator{})
	_, err := service.ImportRemoteYAML(context.Background(), "", remote.URL)
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("ImportRemoteYAML error = %v, want rejected private address", err)
	}
}

func TestRemoteSkillURLValidationRejectsCredentialsAndUnsafeRedirects(t *testing.T) {
	for _, rawURL := range []string{
		"file:///etc/passwd",
		"https://user:password@example.com/skill.yaml",
		"//example.com/skill.yaml",
	} {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse test URL %q: %v", rawURL, err)
		}
		if err := validateRemoteSkillURL(parsed); err == nil {
			t.Fatalf("validateRemoteSkillURL(%q) accepted unsafe URL", rawURL)
		}
	}

	redirect := &http.Request{URL: &url.URL{Scheme: "http", Host: "127.0.0.1", Path: "/private"}}
	if err := remoteSkillHTTPClient.CheckRedirect(redirect, []*http.Request{{}}); err != nil {
		// The redirect hook validates URL shape; the custom dialer rejects the private target
		// after resolution, including redirects and DNS rebinding attempts.
		return
	}
	if isPublicRemoteSkillIP(netip.MustParseAddr("127.0.0.1")) {
		t.Fatal("loopback redirect target was considered public")
	}
}

func TestPublicRemoteSkillIPPolicy(t *testing.T) {
	tests := []struct {
		address string
		public  bool
	}{
		{address: "8.8.8.8", public: true},
		{address: "2606:4700:4700::1111", public: true},
		{address: "127.0.0.1", public: false},
		{address: "10.0.0.1", public: false},
		{address: "169.254.169.254", public: false},
		{address: "100.64.0.1", public: false},
		{address: "::1", public: false},
		{address: "fc00::1", public: false},
		{address: "fe80::1", public: false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := isPublicRemoteSkillIP(netip.MustParseAddr(test.address)); got != test.public {
				t.Fatalf("isPublicRemoteSkillIP(%s) = %v, want %v", test.address, got, test.public)
			}
		})
	}
}

func withRemoteSkillHTTPClient(t *testing.T, status int, body string) {
	t.Helper()
	originalClient := remoteSkillHTTPClient
	remoteSkillHTTPClient = &http.Client{Transport: skillRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	t.Cleanup(func() { remoteSkillHTTPClient = originalClient })
}

type skillRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn skillRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func agentWithSkills(id, skillID string) domain.Agent {
	return domain.Agent{ID: id, ProjectID: "project_1", SkillIDs: []string{skillID}}
}

func testPromptTask(id string) domain.Task {
	return domain.Task{ID: id, Prompt: "Use the imported skill."}
}

func testPromptRun(id string) domain.Run {
	return domain.Run{ID: id}
}
