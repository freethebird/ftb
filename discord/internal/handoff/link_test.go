package handoff

import (
	"net/url"
	"strings"
	"testing"
)

func fragmentValues(t *testing.T, u, wantPath string) url.Values {
	t.Helper()
	hash := u
	if i := strings.Index(u, "#"); i >= 0 {
		hash = u[i+1:]
	} else {
		t.Fatalf("url %q has no fragment", u)
	}
	parts := strings.SplitN(hash, "?", 2)
	if parts[0] != wantPath {
		t.Fatalf("fragment path = %q, want %q", parts[0], wantPath)
	}
	if len(parts) < 2 {
		return url.Values{}
	}
	v, err := url.ParseQuery(parts[1])
	if err != nil {
		t.Fatalf("parse fragment query: %v", err)
	}
	return v
}

func TestNewBuilderTrimsSlash(t *testing.T) {
	b := NewBuilder("https://freethebird.co/sign/")
	if !strings.HasPrefix(b.FileCaseURL("1", ReasonSpamRate, "deadbeef", ""), "https://freethebird.co/sign#/file-case?") {
		t.Fatalf("trailing slash not trimmed: %s", b.FileCaseURL("1", ReasonSpamRate, "deadbeef", ""))
	}
}

func TestFileCaseURL(t *testing.T) {
	b := NewBuilder("https://freethebird.co/sign")
	u := b.FileCaseURL("42", ReasonInviteSpam, "abc123", "ff00")
	v := fragmentValues(t, u, "/file-case")
	if v.Get("accused") != "42" {
		t.Fatalf("accused = %q", v.Get("accused"))
	}
	if v.Get("reason") != "5" { // ReasonInviteSpam
		t.Fatalf("reason = %q, want 5", v.Get("reason"))
	}
	if v.Get("evidence") != "abc123" {
		t.Fatalf("evidence = %q", v.Get("evidence"))
	}
	if v.Get("nonce") != "ff00" {
		t.Fatalf("nonce = %q", v.Get("nonce"))
	}
}

func TestFileCaseURLNoNonce(t *testing.T) {
	b := NewBuilder("https://freethebird.co/sign")
	u := b.FileCaseURL("7", ReasonSpamDup, "aa", "")
	v := fragmentValues(t, u, "/file-case")
	if v.Has("nonce") {
		t.Fatalf("nonce should be absent when empty, got %q", v.Get("nonce"))
	}
}

func TestVoteURL(t *testing.T) {
	b := NewBuilder("https://freethebird.co/sign")
	u := b.VoteURL("99", ChoiceLiquidate)
	v := fragmentValues(t, u, "/vote")
	if v.Get("case") != "99" || v.Get("choice") != "liquidate" {
		t.Fatalf("vote url params wrong: %v", v)
	}
}

func TestConfigured(t *testing.T) {
	if NewBuilder("").Configured() {
		t.Fatal("empty base should be unconfigured")
	}
	if !NewBuilder("https://x").Configured() {
		t.Fatal("non-empty base should be configured")
	}
}

func TestReasonString(t *testing.T) {
	if ReasonInviteSpam.String() != "invite spam" {
		t.Fatalf("unexpected reason string %q", ReasonInviteSpam.String())
	}
}
