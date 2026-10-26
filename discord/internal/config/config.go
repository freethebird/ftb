// Package config loads operational configuration (JSON file) and secrets
// (environment variables) for the two FTB Discord bots.
//
// Canon mapping (Modus Operandi, backend analog of the web R-rules):
//   - No keys in config or code. Bot tokens come from the environment only;
//     they are never written to the JSON config or logged.
//   - Earned-or-standby (R5/R8): when a contract hash is empty the bot runs in
//     an explicit, labelled standby mode instead of faking a result.
//   - One responsibility per field; everything a bot needs is declared here so
//     nothing is hard-coded at a call site.
//
// This package is standard-library only on purpose (dependency minimalism).
// Hash strings are validated for shape by the neo layer, not here.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Environment variables that carry secrets. These are never placed in the
// JSON config file.
const (
	EnvVerifyToken   = "FTB_VERIFY_BOT_TOKEN"   // #nosec G101 -- env-var NAME, not a secret value
	EnvSentinelToken = "FTB_SENTINEL_BOT_TOKEN" // #nosec G101 -- env-var NAME, not a secret value
	EnvConfigPath    = "FTB_CONFIG"
)

// Config is the full operational configuration shared by both bots. Secrets
// are deliberately absent — see Secrets.
type Config struct {
	Discord   DiscordConfig   `json:"discord"`
	Neo       NeoConfig       `json:"neo"`
	Contracts ContractsConfig `json:"contracts"`
	Handoff   HandoffConfig   `json:"handoff"`
	Spam      SpamConfig      `json:"spam"`
	Store     StoreConfig     `json:"store"`
}

// DiscordConfig holds guild, role and channel identifiers (snowflakes as
// strings). Role management lives only with the verify bot; the sentinel bot
// never needs the BondedRoleID.
type DiscordConfig struct {
	GuildID                    string   `json:"guild_id"`
	BondedRoleID               string   `json:"bonded_role_id"`
	BondedMembersChannelID     string   `json:"bonded_members_channel_id"`
	LiquidationBallotChannelID string   `json:"liquidation_ballot_channel_id"`
	ModAlertChannelID          string   `json:"mod_alert_channel_id"`
	VerifyCommandName          string   `json:"verify_command_name"`
	IgnoreRoleIDs              []string `json:"ignore_role_ids"` // roles the sentinel never flags (mods/bots)
}

// NeoConfig describes the read-only Neo N3 RPC seam. No wallet, no signer.
type NeoConfig struct {
	RPCEndpoint   string `json:"rpc_endpoint"`
	FTBTokenHash  string `json:"ftb_token_hash"` // NEP-17 contract of the FTB token
	FTBDecimals   int    `json:"ftb_decimals"`
	TimeoutMillis int    `json:"timeout_millis"`
}

// Timeout returns the RPC timeout as a duration.
func (n NeoConfig) Timeout() time.Duration {
	if n.TimeoutMillis <= 0 {
		return 8 * time.Second
	}
	return time.Duration(n.TimeoutMillis) * time.Millisecond
}

// ContractsConfig holds the interface-first contract wiring. When VaultHash or
// BallotHash is empty the corresponding feature is in standby.
type ContractsConfig struct {
	VaultHash  string `json:"vault_hash"`  // Discord bond-vault contract (empty => standby)
	BallotHash string `json:"ballot_hash"` // accusation-ballot contract (empty => standby)

	// BondAmount is the fixed bond in whole FTB (e.g. "100"). It is compared
	// against the on-chain amount once the vault is live.
	BondAmount string `json:"bond_amount"`

	// Method names are overridable so the bots can be pointed at the real
	// contract once its manifest is final without a code change (R5).
	Methods MethodNames `json:"methods"`
}

// MethodNames is the read-only method surface the bots call. Defaults match the
// interface spec in the README; override per the deployed manifest.
type MethodNames struct {
	GetBond          string `json:"get_bond"`            // (bondId) -> Bond struct
	GetBondByDiscord string `json:"get_bond_by_discord"` // (discordUserId) -> bondId (0 if none)
	BondAmount       string `json:"bond_amount"`         // () -> int (base units)
	GetCase          string `json:"get_case"`            // (caseId) -> Case struct
	ListOpenCases    string `json:"list_open_cases"`     // () -> []caseId
	IsBondedVoter    string `json:"is_bonded_voter"`     // (walletScriptHash) -> bool
	FileCase         string `json:"file_case"`           // (accusedBondId, reason, evidence) -> caseId [member-signed]
	Vote             string `json:"vote"`                // (caseId, choice) -> bool                    [member-signed]
}

// HandoffConfig configures the keyless sign-to-file deep links. The bot never
// signs; it builds a URL to the member-operated signing page.
type HandoffConfig struct {
	SignBaseURL string `json:"sign_base_url"` // e.g. https://freethebird.co/sign
}

// StoreConfig configures the local Discord<->bond cache.
type StoreConfig struct {
	Path           string `json:"path"`            // local JSON cache (verify bot writes, atomic)
	RefreshSeconds int    `json:"refresh_seconds"` // chain re-index interval for the in-memory index
}

// RefreshInterval returns the chain re-index interval.
func (s StoreConfig) RefreshInterval() time.Duration {
	if s.RefreshSeconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(s.RefreshSeconds) * time.Second
}

// SpamConfig holds the configurable spam heuristics. Every rule contributes a
// weighted signal; a message (or a short window of messages) that reaches
// ScoreThreshold is flagged. All rules can be tuned without a code change.
type SpamConfig struct {
	WindowSeconds int `json:"window_seconds"` // sliding window for per-user accounting

	RateMessages int     `json:"rate_messages"` // messages within window that trip the rate rule
	RateWeight   float64 `json:"rate_weight"`

	DuplicateCount      int     `json:"duplicate_count"`      // near-duplicate messages within window
	DuplicateSimilarity float64 `json:"duplicate_similarity"` // 0..1 Jaccard shingle similarity
	DuplicateWeight     float64 `json:"duplicate_weight"`

	MaxMentions   int     `json:"max_mentions"` // user+role mentions in one message
	MentionWeight float64 `json:"mention_weight"`

	MaxLinks     int     `json:"max_links"` // URLs in one message
	LinkWeight   float64 `json:"link_weight"`
	InviteWeight float64 `json:"invite_weight"` // extra weight for discord invite links

	JoinGraceMinutes int     `json:"join_grace_minutes"` // "new account" window after join
	JoinBurstWeight  float64 `json:"join_burst_weight"`

	ScoreThreshold float64 `json:"score_threshold"` // flag when accumulated score >= this

	// AutoModerate, when true, lets the sentinel apply a timeout/delete itself
	// (needs Moderate Members). Default false keeps the sentinel read+post only
	// (least privilege / stealth posture): it detects and posts, humans act.
	AutoModerate       bool `json:"auto_moderate"`
	AutoTimeoutMinutes int  `json:"auto_timeout_minutes"`
	DeleteFlaggedMsgs  bool `json:"delete_flagged_messages"`
}

// Secrets carries the bot tokens read from the environment.
type Secrets struct {
	VerifyToken   string
	SentinelToken string
}

// LoadSecrets reads bot tokens from the environment. Each bot validates only
// the token it needs via RequireVerify / RequireSentinel.
func LoadSecrets() Secrets {
	return Secrets{
		VerifyToken:   strings.TrimSpace(os.Getenv(EnvVerifyToken)),
		SentinelToken: strings.TrimSpace(os.Getenv(EnvSentinelToken)),
	}
}

// RequireVerify returns the verify-bot token or an error naming the env var.
func (s Secrets) RequireVerify() (string, error) {
	if s.VerifyToken == "" {
		return "", fmt.Errorf("missing %s in environment", EnvVerifyToken)
	}
	return s.VerifyToken, nil
}

// RequireSentinel returns the sentinel-bot token or an error naming the env var.
func (s Secrets) RequireSentinel() (string, error) {
	if s.SentinelToken == "" {
		return "", fmt.Errorf("missing %s in environment", EnvSentinelToken)
	}
	return s.SentinelToken, nil
}

// Load reads and validates the JSON config. If path is empty it falls back to
// $FTB_CONFIG and then ./config.json. Defaults are applied for any unset field
// so config.example.json can stay minimal.
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv(EnvConfigPath)
	}
	if path == "" {
		path = "config.json"
	}
	raw, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied config path (FTB_CONFIG env / -config flag), not attacker input
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Discord.VerifyCommandName == "" {
		c.Discord.VerifyCommandName = "verify"
	}
	if c.Neo.FTBDecimals == 0 {
		c.Neo.FTBDecimals = 8
	}
	if c.Contracts.BondAmount == "" {
		c.Contracts.BondAmount = "1000"
	}
	m := &c.Contracts.Methods
	setIfEmpty(&m.GetBond, "getBond")
	setIfEmpty(&m.GetBondByDiscord, "getBondByDiscord")
	setIfEmpty(&m.BondAmount, "bondAmount")
	setIfEmpty(&m.GetCase, "getCase")
	setIfEmpty(&m.ListOpenCases, "listOpenCases")
	setIfEmpty(&m.IsBondedVoter, "isBondedVoter")
	setIfEmpty(&m.FileCase, "fileCase")
	setIfEmpty(&m.Vote, "vote")

	if c.Store.Path == "" {
		c.Store.Path = "ftb-bonds.json"
	}

	s := &c.Spam
	if s.WindowSeconds == 0 {
		s.WindowSeconds = 10
	}
	if s.RateMessages == 0 {
		s.RateMessages = 6
	}
	if s.RateWeight == 0 {
		s.RateWeight = 1.0
	}
	if s.DuplicateCount == 0 {
		s.DuplicateCount = 3
	}
	if s.DuplicateSimilarity == 0 {
		s.DuplicateSimilarity = 0.85
	}
	if s.DuplicateWeight == 0 {
		s.DuplicateWeight = 1.0
	}
	if s.MaxMentions == 0 {
		s.MaxMentions = 5
	}
	if s.MentionWeight == 0 {
		s.MentionWeight = 1.0
	}
	if s.MaxLinks == 0 {
		s.MaxLinks = 4
	}
	if s.LinkWeight == 0 {
		s.LinkWeight = 0.5
	}
	if s.InviteWeight == 0 {
		s.InviteWeight = 1.0
	}
	if s.JoinGraceMinutes == 0 {
		s.JoinGraceMinutes = 10
	}
	if s.JoinBurstWeight == 0 {
		s.JoinBurstWeight = 1.0
	}
	if s.ScoreThreshold == 0 {
		s.ScoreThreshold = 2.0
	}
	if s.AutoTimeoutMinutes == 0 {
		s.AutoTimeoutMinutes = 60
	}
}

func setIfEmpty(p *string, v string) {
	if *p == "" {
		*p = v
	}
}

func (c *Config) validate() error {
	var errs []string
	if c.Discord.GuildID == "" {
		errs = append(errs, "discord.guild_id is required")
	}
	if c.Neo.RPCEndpoint == "" {
		errs = append(errs, "neo.rpc_endpoint is required")
	}
	// Contract hashes may be empty (standby) — not an error. But if a hash is
	// set it must be shaped like a Neo N3 script hash (0x + 40 hex).
	if c.Contracts.VaultHash != "" && !looksLikeScriptHash(c.Contracts.VaultHash) {
		errs = append(errs, "contracts.vault_hash must be 0x + 40 hex")
	}
	if c.Contracts.BallotHash != "" && !looksLikeScriptHash(c.Contracts.BallotHash) {
		errs = append(errs, "contracts.ballot_hash must be 0x + 40 hex")
	}
	if c.Neo.FTBTokenHash != "" && !looksLikeScriptHash(c.Neo.FTBTokenHash) {
		errs = append(errs, "neo.ftb_token_hash must be 0x + 40 hex")
	}
	if len(errs) > 0 {
		return errors.New("invalid config: " + strings.Join(errs, "; "))
	}
	return nil
}

// VaultConfigured reports whether the bond vault is live (not standby).
func (c *Config) VaultConfigured() bool { return c.Contracts.VaultHash != "" }

// BallotConfigured reports whether the accusation ballot is live (not standby).
func (c *Config) BallotConfigured() bool { return c.Contracts.BallotHash != "" }

// looksLikeScriptHash is a shape check only (not a checksum/decode — the neo
// layer decodes for real).
func looksLikeScriptHash(s string) bool {
	s = strings.ToLower(s)
	if !strings.HasPrefix(s, "0x") {
		return false
	}
	s = s[2:]
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
