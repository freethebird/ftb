// Command verifybot is the FTB bond verification bot.
//
// A user runs /verify and fills a modal with the PUBLIC data of their bond
// deposit (tx id, wallet, bond id). The bot checks that data against the
// bond-vault contract on-chain (read-only) and, on a match, grants the Bonded
// role and announces the new bonded member. It handles only public data, holds
// no key, and never moves a bond.
//
// Privilege: this bot needs Manage Roles (to grant the Bonded role). The Bonded
// role must sit BELOW the bot's own top role. It does not read message content.
//
// Standby: if the vault contract hash is unset the bot still runs and validates
// input, but tells the user verification is not live yet (earned-or-standby).
package main

import (
	"context"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/nspcc-dev/neo-go/pkg/util"

	"github.com/freethebird/ftb/discord/internal/config"
	"github.com/freethebird/ftb/discord/internal/neo"
	"github.com/freethebird/ftb/discord/internal/store"
)

const modalID = "ftb_verify_modal"

type app struct {
	cfg   *config.Config
	store *store.Store

	chain      *neo.Client // nil if chain unreachable / not needed
	vaultHash  util.Uint160
	ftbToken   util.Uint160
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

	// Dial the chain. Required when the vault is live; best-effort in standby.
	if cfg.VaultConfigured() {
		if a.vaultHash, err = neo.ParseScriptHash(cfg.Contracts.VaultHash); err != nil {
			log.Fatalf("vault hash: %v", err)
		}
		if cfg.Neo.FTBTokenHash != "" {
			if a.ftbToken, err = neo.ParseScriptHash(cfg.Neo.FTBTokenHash); err != nil {
				log.Fatalf("ftb token hash: %v", err)
			}
		}
	}
	// The chain is only needed once the vault is live; in standby we skip the
	// dial entirely (no RPC, nothing to read).
	if cfg.VaultConfigured() {
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
		Description: "Verify your 100 FTB bond deposit to get the Bonded role",
	}
	if _, err := dg.ApplicationCommandCreate(dg.State.User.ID, cfg.Discord.GuildID, cmd); err != nil {
		log.Fatalf("register /%s: %v", cfg.Discord.VerifyCommandName, err)
	}
	log.Printf("/%s registered on guild %s; %d members cached", cfg.Discord.VerifyCommandName, cfg.Discord.GuildID, st.Count())

	waitForSignal()
	log.Printf("shutting down")
}

func (a *app) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if i.ApplicationCommandData().Name == a.cfg.Discord.VerifyCommandName {
			a.showModal(s, i)
		}
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID == modalID {
			a.handleModal(s, i)
		}
	}
}

func (a *app) showModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: modalID,
			Title:    "Verify your FTB bond",
			Components: []discordgo.MessageComponent{
				row(&discordgo.TextInput{
					CustomID: "txid", Label: "Deposit transaction id (0x…)",
					Style: discordgo.TextInputShort, Required: true, MinLength: 64, MaxLength: 66,
					Placeholder: "0x…",
				}),
				row(&discordgo.TextInput{
					CustomID: "wallet", Label: "Your wallet address (N…)",
					Style: discordgo.TextInputShort, Required: true, MinLength: 30, MaxLength: 40,
					Placeholder: "N…",
				}),
				row(&discordgo.TextInput{
					CustomID: "bondid", Label: "Bond id",
					Style: discordgo.TextInputShort, Required: true, MinLength: 1, MaxLength: 20,
					Placeholder: "e.g. 42",
				}),
			},
		},
	})
	if err != nil {
		log.Printf("show modal: %v", err)
	}
}

func (a *app) handleModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	u := interactionUser(i)
	if u == nil {
		return
	}
	if !a.lim.allow(u.ID) {
		reply(s, i, "You're going a bit fast — try again in a few seconds.")
		return
	}

	data := i.ModalSubmitData()
	txStr := strings.TrimSpace(modalValue(data, "txid"))
	walletStr := strings.TrimSpace(modalValue(data, "wallet"))
	bondStr := strings.TrimSpace(modalValue(data, "bondid"))

	// Validate shapes first (cheap, no chain).
	txHash, err := neo.ParseTxHash(txStr)
	if err != nil {
		reply(s, i, "That transaction id doesn't look right — paste the deposit tx hash (0x + 64 hex).")
		return
	}
	wallet, err := neo.ParseAddress(walletStr)
	if err != nil {
		reply(s, i, "That wallet address doesn't look like a Neo N3 address (starts with N).")
		return
	}
	bondID, ok := new(big.Int).SetString(bondStr, 10)
	if !ok || bondID.Sign() <= 0 {
		reply(s, i, "Bond id must be a positive number.")
		return
	}

	if !a.cfg.VaultConfigured() {
		reply(s, i, "Thanks — your details look well-formed. The bond vault isn't live yet (standby), so I can't confirm on-chain just now. Re-run /verify once the vault contract is deployed and you'll get the Bonded role.")
		return
	}
	if a.chain == nil {
		reply(s, i, "The chain is briefly unreachable — give it a minute and run /verify again.")
		return
	}

	m := &a.cfg.Contracts.Methods

	bond, found, err := a.chain.GetBond(a.vaultHash, m.GetBond, bondID)
	if err != nil {
		log.Printf("getBond(%s): %v", bondID, err)
		reply(s, i, "I couldn't read the vault just now — try again shortly.")
		return
	}
	if !found {
		reply(s, i, "No active bond with that id exists in the vault.")
		return
	}

	// All checks are on public data and must all pass.
	if bond.Status != neo.BondActive {
		reply(s, i, "That bond is "+bond.Status.String()+", so it can't be used to verify.")
		return
	}
	if !bond.Owner.Equals(wallet) {
		reply(s, i, "That bond isn't owned by the wallet you entered.")
		return
	}
	if bond.DiscordID != u.ID {
		reply(s, i, "That bond is tagged to a different Discord account. Post the bond from this account, or contact a mod.")
		return
	}
	if a.bondAmount.Cmp(bond.AmountBase) != 0 {
		reply(s, i, "That bond isn't the required "+a.cfg.Contracts.BondAmount+" FTB.")
		return
	}
	if !bond.DepositTx.Equals(txHash) {
		reply(s, i, "That transaction id doesn't match the deposit recorded for this bond.")
		return
	}

	// Secondary corroboration (advisory): confirm the deposit tx really moved
	// the bond amount from wallet to vault. Logged, not gating — GetBond is
	// authoritative — but a mismatch is worth surfacing in logs.
	if a.ftbToken != (util.Uint160{}) {
		okTransfer, terr := a.chain.VerifyDepositTransfer(txHash, a.ftbToken, a.vaultHash, wallet, a.bondAmount)
		if terr != nil {
			log.Printf("deposit-transfer check (bond %s): %v", bondID, terr)
		} else if !okTransfer {
			log.Printf("WARNING: bond %s passed getBond but no matching NEP-17 Transfer found in tx %s", bondID, txStr)
		}
	}

	// Grant the role.
	if err := s.GuildMemberRoleAdd(a.cfg.Discord.GuildID, u.ID, a.cfg.Discord.BondedRoleID); err != nil {
		log.Printf("role add for %s: %v", u.ID, err)
		reply(s, i, "You're verified on-chain, but I couldn't assign the Bonded role (a permissions issue). A mod will sort it out.")
		return
	}

	_ = a.store.Put(store.Record{
		DiscordID: u.ID, Username: u.Username, Wallet: walletStr,
		BondID: bondID.String(), TxID: txStr, VerifiedAt: store.NowMillis(),
	})

	a.announce(s, u, bondID.String())
	reply(s, i, "✅ Verified — you now hold the Bonded role. Welcome aboard. Spam while bonded and your 100 FTB can be put to a liquidation vote, so keep it clean.")
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

func row(c discordgo.MessageComponent) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{c}}
}

func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}

func modalValue(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, comp := range data.Components {
		var inner []discordgo.MessageComponent
		switch ar := comp.(type) {
		case *discordgo.ActionsRow:
			inner = ar.Components
		case discordgo.ActionsRow:
			inner = ar.Components
		default:
			continue
		}
		for _, c := range inner {
			switch ti := c.(type) {
			case *discordgo.TextInput:
				if ti.CustomID == customID {
					return ti.Value
				}
			case discordgo.TextInput:
				if ti.CustomID == customID {
					return ti.Value
				}
			}
		}
	}
	return ""
}

func reply(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		log.Printf("reply: %v", err)
	}
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
