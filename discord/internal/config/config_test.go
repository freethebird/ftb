package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefaultsAndStandby(t *testing.T) {
	p := writeCfg(t, `{
		"discord": {"guild_id": "123"},
		"neo": {"rpc_endpoint": "https://rpc.example"}
	}`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Discord.VerifyCommandName != "verify" {
		t.Errorf("default verify command = %q", c.Discord.VerifyCommandName)
	}
	if c.Neo.FTBDecimals != 8 {
		t.Errorf("default decimals = %d", c.Neo.FTBDecimals)
	}
	if c.Contracts.BondAmount != "1000" {
		t.Errorf("default bond amount = %q", c.Contracts.BondAmount)
	}
	if c.Contracts.Methods.GetBond != "getBond" || c.Contracts.Methods.GetBondByDiscord != "getBondByDiscord" {
		t.Errorf("default methods wrong: %+v", c.Contracts.Methods)
	}
	if c.Store.Path == "" || c.Spam.ScoreThreshold == 0 {
		t.Errorf("spam/store defaults not applied")
	}
	if c.VaultConfigured() || c.BallotConfigured() {
		t.Errorf("expected standby (no hashes), got vault=%v ballot=%v", c.VaultConfigured(), c.BallotConfigured())
	}
}

func TestLoadLiveVault(t *testing.T) {
	p := writeCfg(t, `{
		"discord": {"guild_id": "123"},
		"neo": {"rpc_endpoint": "https://rpc.example"},
		"contracts": {"vault_hash": "0x1122334455667788990011223344556677889900"}
	}`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !c.VaultConfigured() {
		t.Fatal("vault should be configured")
	}
	if c.BallotConfigured() {
		t.Fatal("ballot should still be standby")
	}
}

func TestRejectBadHash(t *testing.T) {
	p := writeCfg(t, `{
		"discord": {"guild_id": "123"},
		"neo": {"rpc_endpoint": "https://rpc.example"},
		"contracts": {"vault_hash": "nothex"}
	}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for malformed vault hash")
	}
}

func TestRequireGuild(t *testing.T) {
	p := writeCfg(t, `{"neo": {"rpc_endpoint": "https://rpc.example"}}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for missing guild_id")
	}
}

func TestRejectUnknownField(t *testing.T) {
	p := writeCfg(t, `{"discord": {"guild_id": "1"}, "neo": {"rpc_endpoint": "x"}, "bogus": true}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestRequireTokens(t *testing.T) {
	t.Setenv(EnvVerifyToken, "")
	t.Setenv(EnvSentinelToken, "")
	if _, err := LoadSecrets().RequireVerify(); err == nil {
		t.Error("expected missing verify token error")
	}
	t.Setenv(EnvVerifyToken, "abc")
	if tok, err := LoadSecrets().RequireVerify(); err != nil || tok != "abc" {
		t.Errorf("verify token = %q err=%v", tok, err)
	}
}
