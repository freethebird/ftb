// Command sentinelbot watches the server for spam and, when a BONDED member
// spams, assembles an accusation and posts it to the liquidation-ballot channel
// with a keyless "file this case" link. The bot never signs: a bonded member
// opens the link and signs the fileCase transaction with their own wallet on
// the signing page. Non-bonded spammers (no bond to liquidate) are surfaced to
// a mod-alert channel instead.
//
// Privilege (least): Guilds + Guild Messages + Message Content (to read text).
// It does NOT need Manage Roles. Auto-moderation (timeout/delete) is OFF by
// default; enabling it requires Moderate Members and is opt-in via config.
//
// Standby: detection is always live. The on-chain accusation path is
// earned-or-standby — if the ballot/vault or signing page aren't configured,
// the bot still detects and reports, and labels the parts that aren't live yet.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/nspcc-dev/neo-go/pkg/util"

	"github.com/freethebird/ftb/discord/internal/config"
	"github.com/freethebird/ftb/discord/internal/handoff"
	"github.com/freethebird/ftb/discord/internal/neo"
	"github.com/freethebird/ftb/discord/internal/spam"
)

const reportCooldown = 10 * time.Minute

type sentinel struct {
	cfg    *config.Config
	det    *spam.Detector
	links  handoff.Builder
	ignore map[string]struct{}

	chain      *neo.Client
	vaultHash  util.Uint160
	ballotHash util.Uint160

	mu     sync.Mutex
	recent map[string]time.Time // userID -> last report time (dedupe)
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load(os.Getenv(config.EnvConfigPath))
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	token, err := config.LoadSecrets().RequireSentinel()
	if err != nil {
		log.Fatalf("secrets: %v", err)
	}

	s := &sentinel{
		cfg:    cfg,
		det:    spam.New(cfg.Spam),
		links:  handoff.NewBuilder(cfg.Handoff.SignBaseURL),
		ignore: toSet(cfg.Discord.IgnoreRoleIDs),
		recent: map[string]time.Time{},
	}

	if cfg.VaultConfigured() {
		if s.vaultHash, err = neo.ParseScriptHash(cfg.Contracts.VaultHash); err != nil {
			log.Fatalf("vault hash: %v", err)
		}
	}
	if cfg.BallotConfigured() {
		if s.ballotHash, err = neo.ParseScriptHash(cfg.Contracts.BallotHash); err != nil {
			log.Fatalf("ballot hash: %v", err)
		}
	}
	if cfg.VaultConfigured() || cfg.BallotConfigured() {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Neo.Timeout())
		if cl, derr := neo.Dial(ctx, cfg.Neo.RPCEndpoint, cfg.Neo.Timeout()); derr != nil {
			log.Printf("WARNING: RPC %s unreachable (%v); bonded-member resolution will fall back to mod alerts until it recovers", cfg.Neo.RPCEndpoint, derr)
		} else {
			s.chain = cl
			log.Printf("connected to Neo RPC %s", cfg.Neo.RPCEndpoint)
		}
		cancel()
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}
	// Message Content is a privileged intent — enable it for this bot in the
	// Discord developer portal.
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	dg.AddHandler(func(_ *discordgo.Session, r *discordgo.Ready) {
		log.Printf("sentinelbot ready as %s (vault=%s ballot=%s handoff=%s automod=%v)",
			r.User.Username, live(cfg.VaultConfigured()), live(cfg.BallotConfigured()), live(s.links.Configured()), cfg.Spam.AutoModerate)
	})
	dg.AddHandler(s.onMessage)

	if err := dg.Open(); err != nil {
		log.Fatalf("open gateway: %v", err)
	}
	defer dg.Close()

	log.Printf("sentinel watching guild %s", cfg.Discord.GuildID)
	waitForSignal()
	log.Printf("shutting down")
}

func (s *sentinel) onMessage(ds *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore DMs, self, bots, webhooks, and ignored roles.
	if m.GuildID == "" || m.Author == nil || m.Author.Bot || m.WebhookID != "" {
		return
	}
	if ds.State != nil && ds.State.User != nil && m.Author.ID == ds.State.User.ID {
		return
	}
	if m.Member != nil && s.hasIgnoredRole(m.Member.Roles) {
		return
	}

	mentions := len(m.Mentions) + len(m.MentionRoles)
	if m.MentionEveryone {
		mentions += s.cfg.Spam.MaxMentions // @everyone trips the mass-mention rule on its own
	}

	var joined time.Time
	if m.Member != nil {
		joined = m.Member.JoinedAt
	}

	res := s.det.Observe(spam.Message{
		UserID:    m.Author.ID,
		Username:  m.Author.Username,
		GuildID:   m.GuildID,
		ChannelID: m.ChannelID,
		MessageID: m.ID,
		Content:   m.Content,
		Mentions:  mentions,
		CreatedAt: time.Now(),
		JoinedAt:  joined,
	})
	if !res.Flagged {
		return
	}
	if !s.claimReport(m.Author.ID) {
		return // recently reported; don't spam the ballot channel
	}
	log.Printf("flagged %s (%s) score=%.1f reasons=%v", m.Author.Username, m.Author.ID, res.Score, reasonsText(res.Reasons))

	s.handleFlag(ds, m, res)
}

func (s *sentinel) handleFlag(ds *discordgo.Session, m *discordgo.MessageCreate, res spam.Result) {
	bondID, active, known := s.resolveBond(m.Author.ID)

	// Optional, opt-in auto-moderation (default off → least privilege).
	s.maybeAutoModerate(ds, m, res)

	switch {
	case known && active && bondID != nil:
		// Avoid a duplicate accusation if an open case already names this bond.
		if s.hasOpenCaseFor(bondID) {
			log.Printf("bond %s already has an open case; skipping new accusation", bondID)
			s.alertMods(ds, m, res, "bonded member spamming; an open case already exists for bond #"+bondID.String())
			return
		}
		s.postAccusation(ds, m, res, bondID)
	case known && !active:
		// Not bonded: nothing to liquidate. Mods handle it.
		s.alertMods(ds, m, res, "spam from a non-bonded member — no bond to liquidate.")
	default:
		note := "could not resolve bonded status (chain unreachable)."
		if !s.cfg.VaultConfigured() {
			note = "bond vault is in standby — bonded-member resolution isn't live yet."
		}
		s.alertMods(ds, m, res, "spam detected; "+note)
	}
}

// postAccusation posts the keyless accusation card to the liquidation-ballot
// channel: evidence + a Link button to the member-signed file-case page.
func (s *sentinel) postAccusation(ds *discordgo.Session, m *discordgo.MessageCreate, res spam.Result, bondID *big.Int) {
	ch := s.cfg.Discord.LiquidationBallotChannelID
	if ch == "" {
		log.Printf("no liquidation_ballot_channel_id configured; accusation for bond %s not posted", bondID)
		return
	}
	ev := res.Evidence
	primary := handoff.ReasonUnspecified
	if len(res.Reasons) > 0 {
		primary = res.Reasons[0]
	}

	embed := &discordgo.MessageEmbed{
		Title:       "⚖️ Liquidation accusation — bond #" + bondID.String(),
		Description: fmt.Sprintf("Spam from <@%s> while bonded. Bonded members may file this case and vote to **liquidate** (burn the 100 FTB bond) or **dismiss**.", m.Author.ID),
		Color:       0x33ff33,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Reasons", Value: reasonsText(res.Reasons), Inline: false},
			{Name: "Score", Value: fmt.Sprintf("%.1f (threshold %.1f)", res.Score, s.cfg.Spam.ScoreThreshold), Inline: true},
			{Name: "Window msgs", Value: fmt.Sprintf("%d", ev.Counts["window_messages"]), Inline: true},
			{Name: "Evidence hash", Value: "`" + ev.HashHex()[:16] + "…`", Inline: true},
			{Name: "Evidence", Value: truncateField(jumpLinks(ev.Messages)), Inline: false},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: "One bonded member = one vote. No admin key can touch this bond — only the vote decides."},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	msg := &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}}

	if s.links.Configured() {
		url := s.links.FileCaseURL(bondID.String(), primary, ev.HashHex(), nonce())
		msg.Components = []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Style: discordgo.LinkButton, Label: "File this case (sign in your wallet)", URL: url},
			}},
		}
	} else {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name: "Signing page", Value: "standby — not configured yet; case can't be filed until the signing page is live.", Inline: false,
		})
	}

	if _, err := ds.ChannelMessageSendComplex(ch, msg); err != nil {
		log.Printf("post accusation (bond %s): %v", bondID, err)
	}
}

func (s *sentinel) alertMods(ds *discordgo.Session, m *discordgo.MessageCreate, res spam.Result, note string) {
	ch := s.cfg.Discord.ModAlertChannelID
	if ch == "" {
		return
	}
	ev := res.Evidence
	embed := &discordgo.MessageEmbed{
		Title:       "🚨 Spam flagged",
		Description: fmt.Sprintf("<@%s> — %s", m.Author.ID, note),
		Color:       0x888888,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Reasons", Value: reasonsText(res.Reasons), Inline: false},
			{Name: "Score", Value: fmt.Sprintf("%.1f", res.Score), Inline: true},
			{Name: "Evidence", Value: truncateField(jumpLinks(ev.Messages)), Inline: false},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if _, err := ds.ChannelMessageSendComplex(ch, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}}); err != nil {
		log.Printf("alert mods: %v", err)
	}
}

func (s *sentinel) maybeAutoModerate(ds *discordgo.Session, m *discordgo.MessageCreate, res spam.Result) {
	if !s.cfg.Spam.AutoModerate {
		return
	}
	if s.cfg.Spam.DeleteFlaggedMsgs {
		for _, ref := range res.Evidence.Messages {
			if err := ds.ChannelMessageDelete(ref.ChannelID, ref.MessageID); err != nil {
				log.Printf("delete %s: %v", ref.MessageID, err)
			}
		}
	}
	if mins := s.cfg.Spam.AutoTimeoutMinutes; mins > 0 {
		until := time.Now().Add(time.Duration(mins) * time.Minute)
		if err := ds.GuildMemberTimeout(m.GuildID, m.Author.ID, &until); err != nil {
			log.Printf("timeout %s: %v", m.Author.ID, err)
		}
	}
}

// resolveBond resolves a Discord user to their bond and active status from the
// chain. known=false means undetermined (standby or RPC down), handled by a mod
// alert rather than a false negative.
func (s *sentinel) resolveBond(userID string) (bondID *big.Int, active, known bool) {
	if !s.cfg.VaultConfigured() || s.chain == nil {
		return nil, false, false
	}
	mth := &s.cfg.Contracts.Methods
	id, found, err := s.chain.GetBondByDiscord(s.vaultHash, mth.GetBondByDiscord, userID)
	if err != nil {
		log.Printf("getBondByDiscord(%s): %v", userID, err)
		return nil, false, false
	}
	if !found {
		return nil, false, true // known: not bonded
	}
	bond, found2, err := s.chain.GetBond(s.vaultHash, mth.GetBond, id)
	if err != nil {
		log.Printf("getBond(%s): %v", id, err)
		return id, false, false
	}
	if !found2 {
		return id, false, true
	}
	return id, bond.Status == neo.BondActive, true
}

// hasOpenCaseFor best-effort checks the ballot for an existing open case naming
// this bond, to avoid duplicate accusations. Errors are swallowed (non-gating).
func (s *sentinel) hasOpenCaseFor(bondID *big.Int) bool {
	if !s.cfg.BallotConfigured() || s.chain == nil {
		return false
	}
	mth := &s.cfg.Contracts.Methods
	ids, err := s.chain.ListOpenCases(s.ballotHash, mth.ListOpenCases)
	if err != nil {
		return false
	}
	for _, id := range ids {
		cs, found, err := s.chain.GetCase(s.ballotHash, mth.GetCase, id)
		if err != nil || !found {
			continue
		}
		if cs.Status == neo.CaseOpen && cs.AccusedBondID != nil && cs.AccusedBondID.Cmp(bondID) == 0 {
			return true
		}
	}
	return false
}

// ---- helpers ----

func (s *sentinel) hasIgnoredRole(roles []string) bool {
	for _, r := range roles {
		if _, ok := s.ignore[r]; ok {
			return true
		}
	}
	return false
}

func (s *sentinel) claimReport(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if t, ok := s.recent[userID]; ok && now.Sub(t) < reportCooldown {
		return false
	}
	s.recent[userID] = now
	return true
}

func toSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

func reasonsText(rs []handoff.Reason) string {
	if len(rs) == 0 {
		return "—"
	}
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += ", "
		}
		out += r.String()
	}
	return out
}

func jumpLinks(refs []spam.MessageRef) string {
	if len(refs) == 0 {
		return "—"
	}
	max := 5
	out := ""
	for i, ref := range refs {
		if i >= max {
			out += fmt.Sprintf("\n…and %d more", len(refs)-max)
			break
		}
		out += fmt.Sprintf("• [msg %d](%s)\n", i+1, ref.JumpURL())
	}
	return out
}

func truncateField(s string) string {
	const max = 1024 // Discord embed field value limit
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func nonce() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func live(b bool) string {
	if b {
		return "live"
	}
	return "standby"
}

func waitForSignal() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
