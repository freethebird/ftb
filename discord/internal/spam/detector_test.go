package spam

import (
	"testing"
	"time"

	"github.com/freethebird/ftb/discord/internal/config"
	"github.com/freethebird/ftb/discord/internal/handoff"
)

func testCfg() config.SpamConfig {
	return config.SpamConfig{
		WindowSeconds:       10,
		RateMessages:        5,
		RateWeight:          1,
		DuplicateCount:      3,
		DuplicateSimilarity: 0.8,
		DuplicateWeight:     1,
		MaxMentions:         5,
		MentionWeight:       1,
		MaxLinks:            4,
		LinkWeight:          0.5,
		InviteWeight:        1,
		JoinGraceMinutes:    10,
		JoinBurstWeight:     1,
		ScoreThreshold:      2,
	}
}

func hasReason(rs []handoff.Reason, want handoff.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

func TestRatePlusDuplicateFlags(t *testing.T) {
	d := New(testCfg())
	base := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	msg := func(n int) Message {
		return Message{
			UserID: "u1", Username: "spammer", GuildID: "g", ChannelID: "c",
			MessageID: itoa(n), Content: "buy cheap tokens now click here",
			CreatedAt: base.Add(time.Duration(n) * time.Second),
		}
	}
	// Messages 1..4 must not flag; the 5th trips rate(>=5)+duplicate.
	for n := 1; n <= 4; n++ {
		if r := d.Observe(msg(n)); r.Flagged {
			t.Fatalf("message %d flagged too early (score %.1f)", n, r.Score)
		}
	}
	r := d.Observe(msg(5))
	if !r.Flagged {
		t.Fatalf("5th identical message not flagged (score %.1f)", r.Score)
	}
	if !hasReason(r.Reasons, handoff.ReasonSpamRate) || !hasReason(r.Reasons, handoff.ReasonSpamDup) {
		t.Fatalf("expected rate+duplicate reasons, got %v", r.Reasons)
	}
	if len(r.Evidence.Messages) == 0 {
		t.Fatalf("evidence has no message refs")
	}
}

func TestMentionLinkInviteFlagsSingleMessage(t *testing.T) {
	d := New(testCfg())
	r := d.Observe(Message{
		UserID: "u2", Username: "ad", GuildID: "g", ChannelID: "c", MessageID: "1",
		Content:   "join https://discord.gg/abc http://x.com http://y.com http://z.com http://w.com",
		Mentions:  5, // simulates @everyone / mass mention
		CreatedAt: time.Now(),
	})
	if !r.Flagged {
		t.Fatalf("mass-mention + link-flood + invite not flagged (score %.1f)", r.Score)
	}
	for _, want := range []handoff.Reason{handoff.ReasonMassMention, handoff.ReasonLinkFlood, handoff.ReasonInviteSpam} {
		if !hasReason(r.Reasons, want) {
			t.Fatalf("missing reason %v in %v", want, r.Reasons)
		}
	}
}

func TestJoinBurstReason(t *testing.T) {
	d := New(testCfg())
	base := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	joined := base.Add(-5 * time.Minute) // within 10-minute grace
	for n := 1; n <= 5; n++ {
		r := d.Observe(Message{
			UserID: "u3", Username: "fresh", GuildID: "g", ChannelID: "c", MessageID: itoa(n),
			Content: "spam spam spam spam", CreatedAt: base.Add(time.Duration(n) * time.Second), JoinedAt: joined,
		})
		if n == 5 {
			if !r.Flagged || !hasReason(r.Reasons, handoff.ReasonJoinBurst) {
				t.Fatalf("expected join-burst flag on 5th msg, got flagged=%v reasons=%v", r.Flagged, r.Reasons)
			}
		}
	}
}

func TestNormalChatNotFlagged(t *testing.T) {
	d := New(testCfg())
	now := time.Now()
	for i, c := range []string{"hey everyone how's it going", "working on the neo contract today", "anyone tried the testnet rpc"} {
		r := d.Observe(Message{
			UserID: "u4", Username: "regular", GuildID: "g", ChannelID: "c", MessageID: itoa(i),
			Content: c, CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
		if r.Flagged {
			t.Fatalf("normal message %q flagged (score %.1f, reasons %v)", c, r.Score, r.Reasons)
		}
	}
}

func TestEvidenceHashDeterministicAndHex(t *testing.T) {
	ev := Evidence{
		UserID: "u1", Username: "x",
		Reasons:  []handoff.Reason{handoff.ReasonSpamRate, handoff.ReasonSpamDup},
		Score:    2,
		Counts:   map[string]int{"window_messages": 5, "duplicates": 5},
		Messages: []MessageRef{{GuildID: "g", ChannelID: "c", MessageID: "1"}},
		Sample:   "buy cheap tokens now",
	}
	h1 := ev.HashHex()
	h2 := ev.HashHex()
	if h1 != h2 {
		t.Fatalf("evidence hash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("evidence hash not 32-byte hex: len %d", len(h1))
	}
}

func TestJaccard(t *testing.T) {
	a := tokenSet(normalize("buy cheap tokens now"))
	b := tokenSet(normalize("buy cheap tokens now!!!"))
	if s := jaccard(a, b); s < 0.99 {
		t.Fatalf("expected near-identical similarity ~1.0, got %.2f", s)
	}
	c := tokenSet(normalize("completely different sentence here"))
	if s := jaccard(a, c); s > 0.2 {
		t.Fatalf("expected low similarity, got %.2f", s)
	}
}

// itoa is a tiny test helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
