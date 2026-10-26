// Package spam is the sentinel bot's configurable spam-detection engine.
//
// It is pure logic — no Discord, no chain — so it is unit-testable in isolation.
// Each heuristic contributes a weighted signal; when a user's accumulated score
// over a sliding window reaches the threshold the message is flagged and an
// Evidence bundle is assembled (jump links to the offending messages, per-rule
// counts, a content sample, and a deterministic sha-256 used as the on-chain
// evidenceHash).
//
// Every rule and weight is config-driven (config.SpamConfig): the engine encodes
// no fixed policy.
package spam

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/freethebird/ftb/discord/internal/config"
	"github.com/freethebird/ftb/discord/internal/handoff"
)

var (
	reURL    = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	reInvite = regexp.MustCompile(`(?i)\b(?:discord\.gg|discord(?:app)?\.com/invite|discord\.com/invite)/\S+`)
	reWord   = regexp.MustCompile(`[a-z0-9]+`)
)

// Message is one observed Discord message, reduced to what the engine needs.
type Message struct {
	UserID    string
	Username  string
	GuildID   string
	ChannelID string
	MessageID string
	Content   string
	Mentions  int       // user + role mentions in this message (bot computes; @everyone counts high)
	CreatedAt time.Time // message time
	JoinedAt  time.Time // member join time (zero if unknown)
}

// MessageRef points back to a Discord message for a jump link.
type MessageRef struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}

// JumpURL renders the Discord jump link for the message.
func (r MessageRef) JumpURL() string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", r.GuildID, r.ChannelID, r.MessageID)
}

// Evidence is the bundle attached to an accusation. It is public, off-chain
// content; only its sha-256 goes on-chain (evidenceHash).
type Evidence struct {
	UserID   string           `json:"user_id"`
	Username string           `json:"username"`
	Reasons  []handoff.Reason `json:"reasons"`
	Score    float64          `json:"score"`
	Counts   map[string]int   `json:"counts"`
	Messages []MessageRef     `json:"messages"`
	Sample   string           `json:"sample"`
}

// HashHex is the deterministic sha-256 (hex) over a canonical serialization of
// the evidence, used as the on-chain evidenceHash. It is stable regardless of
// map ordering.
func (e Evidence) HashHex() string {
	var b strings.Builder
	b.WriteString("v1\n")
	b.WriteString("user=" + e.UserID + "\n")
	// counts, sorted by key
	keys := make([]string, 0, len(e.Counts))
	for k := range e.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("count.%s=%d\n", k, e.Counts[k]))
	}
	for _, r := range e.Reasons {
		b.WriteString(fmt.Sprintf("reason=%d\n", int(r)))
	}
	for _, m := range e.Messages {
		b.WriteString("msg=" + m.ChannelID + "/" + m.MessageID + "\n")
	}
	b.WriteString("sample=" + e.Sample + "\n")
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Result is the outcome of observing one message.
type Result struct {
	Flagged  bool
	Score    float64
	Reasons  []handoff.Reason
	Evidence Evidence
}

type entry struct {
	norm      string
	tokens    map[string]struct{}
	channelID string
	messageID string
	at        time.Time
	mentions  int
	links     int
	invites   int
}

type userState struct {
	username string
	joinedAt time.Time
	entries  []entry
	lastSeen time.Time
}

// Detector is a concurrency-safe spam detector.
type Detector struct {
	cfg config.SpamConfig
	mu  sync.Mutex
	st  map[string]*userState
}

// New builds a Detector from config.
func New(cfg config.SpamConfig) *Detector {
	return &Detector{cfg: cfg, st: map[string]*userState{}}
}

// Observe records a message and returns whether it (with its recent window)
// constitutes spam.
func (d *Detector) Observe(m Message) Result {
	d.mu.Lock()
	defer d.mu.Unlock()

	window := time.Duration(d.cfg.WindowSeconds) * time.Second
	now := m.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}

	us := d.st[m.UserID]
	if us == nil {
		us = &userState{}
		d.st[m.UserID] = us
	}
	us.username = m.Username
	if !m.JoinedAt.IsZero() {
		us.joinedAt = m.JoinedAt
	}
	us.lastSeen = now

	norm := normalize(m.Content)
	e := entry{
		norm:      norm,
		tokens:    tokenSet(norm),
		channelID: m.ChannelID,
		messageID: m.MessageID,
		at:        now,
		mentions:  m.Mentions,
		links:     len(reURL.FindAllString(m.Content, -1)),
		invites:   len(reInvite.FindAllString(m.Content, -1)),
	}

	// prune window, then append
	cutoff := now.Add(-window)
	kept := us.entries[:0]
	for _, old := range us.entries {
		if old.at.After(cutoff) {
			kept = append(kept, old)
		}
	}
	us.entries = append(kept, e)

	d.sweepLocked(now, window)

	// ---- signals ----
	cfg := d.cfg
	score := 0.0
	var reasons []handoff.Reason
	counts := map[string]int{}

	rate := len(us.entries)
	counts["window_messages"] = rate
	if rate >= cfg.RateMessages {
		score += cfg.RateWeight
		reasons = append(reasons, handoff.ReasonSpamRate)
	}

	// duplicate / near-duplicate cluster for the current message
	dup := 1 // the current message
	if len(e.tokens) > 0 {
		for i := 0; i < len(us.entries)-1; i++ {
			if jaccard(e.tokens, us.entries[i].tokens) >= cfg.DuplicateSimilarity {
				dup++
			}
		}
	}
	counts["duplicates"] = dup
	if dup >= cfg.DuplicateCount {
		score += cfg.DuplicateWeight
		reasons = append(reasons, handoff.ReasonSpamDup)
	}

	counts["mentions"] = e.mentions
	if e.mentions >= cfg.MaxMentions {
		score += cfg.MentionWeight
		reasons = append(reasons, handoff.ReasonMassMention)
	}

	counts["links"] = e.links
	if e.links >= cfg.MaxLinks {
		score += cfg.LinkWeight
		reasons = append(reasons, handoff.ReasonLinkFlood)
	}
	counts["invites"] = e.invites
	if e.invites > 0 {
		score += cfg.InviteWeight
		reasons = append(reasons, handoff.ReasonInviteSpam)
	}

	if !us.joinedAt.IsZero() {
		age := now.Sub(us.joinedAt)
		if age <= time.Duration(cfg.JoinGraceMinutes)*time.Minute && rate >= cfg.RateMessages {
			score += cfg.JoinBurstWeight
			reasons = append(reasons, handoff.ReasonJoinBurst)
		}
	}

	reasons = dedupeReasons(reasons)
	res := Result{Score: score, Reasons: reasons}
	if score >= cfg.ScoreThreshold && len(reasons) > 0 {
		res.Flagged = true
		res.Evidence = Evidence{
			UserID:   m.UserID,
			Username: us.username,
			Reasons:  reasons,
			Score:    score,
			Counts:   counts,
			Messages: recentRefs(m.GuildID, us.entries, 10),
			Sample:   truncate(norm, 240),
		}
	}
	return res
}

// sweepLocked bounds memory by dropping user states idle for > 4 windows.
func (d *Detector) sweepLocked(now time.Time, window time.Duration) {
	if len(d.st) < 2048 {
		return
	}
	idle := 4 * window
	for id, us := range d.st {
		if now.Sub(us.lastSeen) > idle {
			delete(d.st, id)
		}
	}
}

func recentRefs(guildID string, entries []entry, max int) []MessageRef {
	start := 0
	if len(entries) > max {
		start = len(entries) - max
	}
	out := make([]MessageRef, 0, len(entries)-start)
	for _, e := range entries[start:] {
		if e.messageID == "" {
			continue
		}
		out = append(out, MessageRef{GuildID: guildID, ChannelID: e.channelID, MessageID: e.messageID})
	}
	return out
}

// --- text helpers ---

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSpace = false
		} else {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func tokenSet(norm string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range reWord.FindAllString(norm, -1) {
		m[w] = struct{}{}
	}
	return m
}

// jaccard is the token-set Jaccard similarity (0..1).
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	// iterate the smaller set
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	for k := range small {
		if _, ok := large[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func dedupeReasons(rs []handoff.Reason) []handoff.Reason {
	seen := map[handoff.Reason]struct{}{}
	out := rs[:0]
	for _, r := range rs {
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
