// Command verifybot is the FTB bond verification bot.
//
// A member runs /verify; the bot looks up their bond by their Discord user ID
// in the bond-vault contract (read-only) and, on an active match, grants the
// Bonded role and announces the new member. No input, no paste: the bond is
// tagged to the member's immutable Discord ID at deposit, and a slash command
// is cryptographically from that member, so the ID itself is the proof. The bot
// holds no key and never moves a bond.
//
// Privilege: this bot needs Manage Roles (to grant the Bonded role). The Bonded
// role must sit BELOW the bot's own top role. It does not read message content.
//
// Standby: if the vault contract hash is unset the bot still runs but tells the
// member verification isn't live yet (earned-or-standby).
package main

import (
	"context"
	"log"
	"math/big"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/util"

	"github.com/freethebird/ftb/discord/internal/config"
	"github.com/freethebird/ftb/discord/internal/neo"
	"github.com/freethebird/ftb/discord/internal/store"
)

type app struct {
	cfg   *config.Config
	store *store.Store

	chain      *neo.Client // nil if chain unreachable / not needed
	vaultHash  util.Uint160
	bondAmount *big.Int

	lim *limiter
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load(os.Getenv(config.EnvConfigPath))
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	token, err := config.LoadSecrets().RequireVerify()
	if err != nil {
		log.Fatalf("secrets: %v", err)
	}

	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	a := &app{cfg: cfg, store: st, lim: newLimiter(8 * time.Second)}
	a.bondAmount, err = neo.ParseFixed(cfg.Contracts.BondAmount, cfg.Neo.FTBDecimals)
	if err != nil {
		log.Fatalf("bond amount: %v", err)
	}

	// The chain is only needed once the vault is live; in standby we skip the
	// dial entirely (no RPC, nothing to read).
	if cfg.VaultConfigured() {
		if a.vaultHash, err = neo.ParseScriptHash(cfg.Contracts.VaultHash); err != nil {
			log.Fatalf("vault hash: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Neo.Timeout())
		cl, derr := neo.Dial(ctx, cfg.Neo.RPCEndpoint, cfg.Neo.Timeout())
		cancel()
		if derr != nil {
			log.Printf("WARNING: vault is live but RPC %s is unreachable (%v); verification will report a transient error until it recovers", cfg.Neo.RPCEndpoint, derr)
		} else {
			a.chain = cl
			log.Printf("connected to Neo RPC %s", cfg.Neo.RPCEndpoint)
			// Bond size is a governed on-chain variable; prefer the vault's live
			// value over static config so a Ballot that changes it can't desync.
			if amt, aerr := a.chain.BondAmountBase(a.vaultHash, cfg.Contracts.Methods.BondAmount); aerr != nil {
				log.Printf("WARNING: could not read live bondAmount(), using config %s FTB: %v", cfg.Contracts.BondAmount, aerr)
			} else {
				a.bondAmount = amt
				log.Printf("live bond amount from vault = %s base units", amt.String())
			}
		}
	} else {
		log.Printf("standby: vault not configured — chain not dialled")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuilds // no message-content intent needed

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("verifybot ready as %s#%s (vault=%s)", r.User.Username, r.User.Discriminator, standbyLabel(cfg.VaultConfigured()))
	})
	dg.AddHandler(a.onInteraction)

	if err := dg.Open(); err != nil {
		log.Fatalf("open gateway: %v", err)
	}
	defer dg.Close()

	// Register the /verify command on the guild (instant, unlike global).
	cmd := &discordgo.ApplicationCommand{
		Name:        cfg.Discord.VerifyCommandName,
		Description: "Verify your FTB bond and get the Bonded role",
	}
	if _, err := dg.ApplicationCommandCreate(dg.State.User.ID, cfg.Discord.GuildID, cmd); err != nil {
		log.Fatalf("register /%s: %v", cfg.Discord.VerifyCommandName, err)
	}
	log.Printf("/%s registered on guild %s; %d members cached", cfg.Discord.VerifyCommandName, cfg.Discord.GuildID, st.Count())

	waitForSignal()
	log.Printf("shutting down")
}

func (a *app) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.ApplicationCommandData().Name == a.cfg.Discord.VerifyCommandName {
		a.handleVerify(s, i)
	}
}

// handleVerify is the whole /verify flow, zero input. It acknowledges within
// Discord's 3-second window (deferred + ephemeral), does the read-only chain
// work and the role grant, then edits the reply with the outcome.
func (a *app) handleVerify(s *discordgo.Session, i *discordgo.InteractionCreate) {
	u := interactionUser(i)
	if u == nil {
		return
	}
	if !a.lim.allow(u.ID) {
		respondEphemeral(s, i, "You're going a bit fast — try again in a few seconds.")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		log.Printf("defer: %v", err)
		return
	}
	msg := a.verifyAndGrant(s, u)
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg}); err != nil {
		log.Printf("followup edit: %v", err)
	}
}

// verifyAndGrant does the read-only lookup and the role grant, returning the
// member-facing message. Every check is on public, on-chain data.
func (a *app) verifyAndGrant(s *discordgo.Session, u *discordgo.User) string {
	if !a.cfg.VaultConfigured() {
		return "The bond vault isn't live yet (standby). Run /verify once the vault contract is deployed and you'll get the Bonded role."
	}
	if a.chain == nil {
		return "The chain is briefly unreachable — give it a minute and run /verify again."
	}
	m := &a.cfg.Contracts.Methods

	bondID, found, err := a.chain.GetBondByDiscord(a.vaultHash, m.GetBondByDiscord, u.ID)
	if err != nil {
		log.Printf("getBondByDiscord(%s): %v", u.ID, err)
		return "I couldn't read the vault just now — try again shortly."
	}
	if !found {
		return "No active bond is tagged to this Discord account yet. Post your bond on the bond page, then run /verify."
	}

	bond, found, err := a.chain.GetBond(a.vaultHash, m.GetBond, bondID)
	if err != nil {
		log.Printf("getBond(%s): %v", bondID, err)
		return "I couldn't read your bond just now — try again shortly."
	}
	if !found || bond.Status != neo.BondActive {
		return "Your bond isn't active (it may have been refunded or liquidated), so I can't grant the role."
	}
	// The reverse index already keys by Discord ID; confirm it as defence in depth.
	if bond.DiscordID != u.ID {
		return "That bond is tagged to a different Discord account. Contact a mod."
	}
	if a.bondAmount != nil && bond.AmountBase != nil && a.bondAmount.Cmp(bond.AmountBase) != 0 {
		return "Your bond isn't the required " + a.cfg.Contracts.BondAmount + " FTB. Contact a mod."
	}

	if err := s.GuildMemberRoleAdd(a.cfg.Discord.GuildID, u.ID, a.cfg.Discord.BondedRoleID); err != nil {
		log.Printf("role add for %s: %v", u.ID, err)
		return "You're verified on-chain, but I couldn't assign the Bonded role (a permissions issue). A mod will sort it out."
	}

	_ = a.store.Put(store.Record{
		DiscordID:  u.ID,
		Username:   u.Username,
		Wallet:     address.Uint160ToString(bond.Owner),
		BondID:     bondID.String(),
		TxID:       txDisplay(bond.DepositTx),
		VerifiedAt: store.NowMillis(),
	})

	a.announce(s, u, bondID.String())
	return "✅ Verified — you now hold the Bonded role. Welcome aboard. Spam while bonded and your bond can be put to a liquidation vote, so keep it clean."
}

func (a *app) announce(s *discordgo.Session, u *discordgo.User, bondID string) {
	ch := a.cfg.Discord.BondedMembersChannelID
	if ch == "" {
		return
	}
	if _, err := s.ChannelMessageSend(ch, "🪶 <@"+u.ID+"> is now **bonded** — bond #"+bondID+". One member, one vote, one bond at stake."); err != nil {
		log.Printf("announce: %v", err)
	}
}

// ---- helpers ----

func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		log.Printf("respond: %v", err)
	}
}

func txDisplay(h util.Uint256) string {
	var zero util.Uint256
	if h == zero {
		return ""
	}
	return "0x" + h.StringLE()
}

func standbyLabel(configured bool) string {
	if configured {
		return "live"
	}
	return "standby"
}

func waitForSignal() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

// limiter is a tiny per-user cooldown.
type limiter struct {
	mu   sync.Mutex
	last map[string]time.Time
	min  time.Duration
}

func newLimiter(min time.Duration) *limiter {
	return &limiter{last: map[string]time.Time{}, min: min}
}

func (l *limiter) allow(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if t, ok := l.last[id]; ok && now.Sub(t) < l.min {
		return false
	}
	l.last[id] = now
	return true
}
