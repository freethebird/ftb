// ftb-node -- the one-install Free the Bird mirror.
//
// It fetches and supervises Kubo (the IPFS node), pins FTB's front-end by its
// stable IPNS name, auto-follows every new release, and serves your own copy of
// FTB locally. Running it makes your machine one more place FTB lives that no one
// can take down. It holds no keys and signs nothing.
//
// Operators can attach their Discord user ID; the node then publishes a claim signed
// under its own key at /ipns/<nodeID> that the FTB verify bot reads to grant the
// contributor role (on top of the bond) -- proof the node is really yours and mirroring.
//
// Zero third-party dependencies -- standard library only, so the whole thing is
// auditable and reproducible (MO Article 2).
//
// Usage:
//
//	ftb-node                      run the node (fetch Kubo, pin, follow, serve)
//	ftb-node set-discord <id>     attach your Discord user ID (persisted)
//	ftb-node id                   print node identity + Discord ID + release
//	ftb-node verify               check this node is a live provider of the release
//	ftb-node version              print the ftb-node version
//
// Flags (e.g. -data, -discord, -no-serve) may follow the command.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// FTB's stable front-end pointer. The IPNS name always resolves to the latest
// release; pinning and following it is what keeps a mirror current.
const defaultIPNS = "k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq"

const ftbNodeVersion = "0.1.1"

type cfg struct {
	dataDir   string
	ipns      string
	gateway   string // host:port for the local gateway
	interval  time.Duration
	kuboVer   string // empty => resolve latest
	serve     bool
	discordID string
}

func main() {
	known := map[string]bool{"run": true, "id": true, "verify": true, "version": true, "set-discord": true}
	cmd := "run"
	args := os.Args[1:]
	if len(args) > 0 && known[args[0]] {
		cmd = args[0]
		args = args[1:]
	}
	// set-discord takes a leading positional ID; pull it out so flags after it
	// (e.g. -data) still parse (Go's flag parser otherwise stops at a positional).
	var setID string
	if cmd == "set-discord" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		setID = args[0]
		args = args[1:]
	}

	home, _ := os.UserHomeDir()
	c := cfg{}
	fs := flag.NewFlagSet("ftb-node", flag.ExitOnError)
	fs.StringVar(&c.dataDir, "data", filepath.Join(home, ".ftb-node"), "data directory (Kubo binary + repo live here)")
	fs.StringVar(&c.ipns, "ipns", defaultIPNS, "FTB IPNS name to mirror")
	fs.StringVar(&c.gateway, "gateway", "127.0.0.1:8080", "local gateway address (host:port)")
	fs.DurationVar(&c.interval, "interval", 15*time.Minute, "how often to check for a new FTB release")
	fs.StringVar(&c.kuboVer, "kubo", "", "pin a Kubo version (e.g. v0.38.1); empty resolves the latest release")
	discordFlag := fs.String("discord", "", "your Discord user ID (numeric) for the operator verifier")
	noServe := fs.Bool("no-serve", false, "do not serve FTB locally (pin only)")
	_ = fs.Parse(args) // flag.ExitOnError: Parse never returns a non-nil error
	c.serve = !*noServe

	// Discord ID: flag sets+persists; otherwise load the saved one.
	if *discordFlag != "" {
		c.discordID = *discordFlag
	} else {
		c.discordID = loadDiscord(c)
	}

	var err error
	switch cmd {
	case "version":
		fmt.Printf("ftb-node %s (%s/%s)\n", ftbNodeVersion, runtime.GOOS, runtime.GOARCH)
		return
	case "set-discord":
		if setID == "" {
			setID = fs.Arg(0)
		}
		err = setDiscord(c, setID)
	case "run":
		if *discordFlag != "" {
			if serr := saveDiscord(c, *discordFlag); serr != nil {
				logf("could not persist Discord ID: %v", serr)
			}
		}
		err = run(c)
	case "id":
		err = printID(c)
	case "verify":
		err = verify(c)
	default:
		err = fmt.Errorf("unknown command %q (try: run, set-discord, id, verify, version)", cmd)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ftb-node: %v\n", err)
		os.Exit(1)
	}
}

// ---- top-level commands -----------------------------------------------------

func setDiscord(c cfg, id string) error {
	id = strings.TrimSpace(id)
	if err := validDiscord(id); err != nil {
		return err
	}
	if err := os.MkdirAll(c.dataDir, 0o750); err != nil {
		return err
	}
	if err := saveDiscord(c, id); err != nil {
		return err
	}
	fmt.Printf("Saved Discord user ID %s for this node.\n", id)
	fmt.Println("Start the node (ftb-node), then in the FTB Discord run /verify with your Node ID (ftb-node id).")
	return nil
}

func run(c cfg) error {
	logf("Free the Bird node %s starting", ftbNodeVersion)
	ipfsBin, err := ensureKubo(c)
	if err != nil {
		return err
	}
	if err := ensureRepo(c, ipfsBin); err != nil {
		return err
	}
	applyConfig(c, ipfsBin)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	daemon, err := startDaemon(ctx, c, ipfsBin)
	if err != nil {
		return err
	}
	if err := waitReady(c, ipfsBin, 90*time.Second); err != nil {
		return fmt.Errorf("the IPFS daemon did not come up: %w", err)
	}

	peer := firstLine(try(ipfsOut(c, ipfsBin, "id", "-f=<id>")))
	logf("node is up")

	cid, _ := followOnce(c, ipfsBin)
	banner(c, peer)
	publishClaim(c, ipfsBin, peer, cid)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ncid, changed := followOnce(c, ipfsBin); changed {
					publishClaim(c, ipfsBin, peer, ncid)
				}
			}
		}
	}()

	<-ctx.Done()
	logf("shutting down -- stopping the IPFS daemon")
	_ = daemon.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = daemon.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		_ = daemon.Process.Kill()
	}
	logf("stopped. Your pinned copy stays on disk; run ftb-node again to resume mirroring.")
	return nil
}

func printID(c cfg) error {
	ipfsBin := filepath.Join(c.dataDir, "bin", binName())
	if !fileExists(ipfsBin) {
		return errors.New("no node yet -- run `ftb-node` once first")
	}
	peer := firstLine(try(ipfsOut(c, ipfsBin, "id", "-f=<id>")))
	fmt.Printf("Node ID    : %s\n", peer)
	fmt.Printf("Discord ID : %s\n", orUnset(c.discordID))
	fmt.Printf("Release    : %s\n", orDash(readState(c)))
	fmt.Printf("IPNS       : /ipns/%s\n", c.ipns)
	fmt.Printf("Claim      : /ipns/%s  (operator claim the verify bot reads)\n", peer)
	fmt.Println("\nIn the FTB Discord, run /verify and give your Node ID above to be raised to the contributor role (on top of your bond).")
	if c.discordID == "" {
		fmt.Println("First attach your Discord ID:  ftb-node set-discord <your-discord-user-id>")
	}
	return nil
}

func verify(c cfg) error {
	ipfsBin := filepath.Join(c.dataDir, "bin", binName())
	if !fileExists(ipfsBin) {
		return errors.New("no node yet -- run `ftb-node` once first")
	}
	peer := firstLine(try(ipfsOut(c, ipfsBin, "id", "-f=<id>")))
	cid, err := resolve(c, ipfsBin)
	if err != nil {
		return fmt.Errorf("could not resolve the current FTB release (is the node running?): %w", err)
	}
	out, err := ipfsOut(c, ipfsBin, "routing", "findprovs", "--num-providers=50", cid)
	if err != nil {
		return fmt.Errorf("could not query providers (is the node running?): %w", err)
	}
	fmt.Printf("Node ID    : %s\n", peer)
	fmt.Printf("Discord ID : %s\n", orUnset(c.discordID))
	fmt.Printf("Release    : %s\n", cid)
	if strings.Contains(out, peer) {
		fmt.Println("Status     : VERIFIED -- this node is a live provider of the current FTB release.")
	} else {
		fmt.Println("Status     : NOT YET -- not announcing the current release yet (give it a few minutes).")
	}
	return nil
}

// ---- operator claim ---------------------------------------------------------

// publishClaim writes an operator claim and publishes it under the node's own
// IPNS key, so /ipns/<nodeID> resolves to {discordId, peerId, release}. Only this
// node holds its key, so the claim cannot be forged for someone else's node.
func publishClaim(c cfg, ipfsBin, peer, release string) {
	if c.discordID == "" {
		return
	}
	go func() {
		claim := fmt.Sprintf(`{"ftbOperator":true,"discordId":%q,"peerId":%q,"ipns":%q,"release":%q,"ts":%d}`,
			c.discordID, peer, c.ipns, release, time.Now().Unix())
		path := filepath.Join(c.dataDir, "ftb-operator.json")
		if err := os.WriteFile(path, []byte(claim+"\n"), 0o600); err != nil {
			logf("operator claim: %v", err)
			return
		}
		cid, err := ipfsOut(c, ipfsBin, "add", "-Q", path)
		if err != nil {
			logf("operator claim add: %v", err)
			return
		}
		cid = firstLine(cid)
		if _, err := ipfsOut(c, ipfsBin, "name", "publish", "--allow-offline", "/ipfs/"+cid); err != nil {
			logf("operator claim publish: %v -- will retry on the next release", err)
			return
		}
		logf("contributor claim published at /ipns/%s (Discord %s)", peer, c.discordID)
	}()
}

// ---- Kubo acquisition -------------------------------------------------------

func ensureKubo(c cfg) (string, error) {
	bin := filepath.Join(c.dataDir, "bin", binName())
	if fileExists(bin) {
		return bin, nil
	}
	if err := os.MkdirAll(filepath.Join(c.dataDir, "bin"), 0o750); err != nil {
		return "", err
	}
	ver := c.kuboVer
	if ver == "" {
		v, err := latestKubo()
		if err != nil {
			return "", fmt.Errorf("could not resolve the latest Kubo version: %w (pass -kubo vX.Y.Z to pin one)", err)
		}
		ver = v
	}
	if !strings.HasPrefix(ver, "v") {
		ver = "v" + ver
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	asset := fmt.Sprintf("kubo_%s_%s-%s.%s", ver, runtime.GOOS, runtime.GOARCH, ext)
	base := fmt.Sprintf("https://github.com/ipfs/kubo/releases/download/%s/%s", ver, asset)
	logf("installing Kubo %s for %s/%s (one time)", ver, runtime.GOOS, runtime.GOARCH)

	archive := filepath.Join(c.dataDir, "bin", asset)
	if err := download(base, archive); err != nil {
		return "", fmt.Errorf("download Kubo: %w", err)
	}
	defer os.Remove(archive)

	// Verify SHA-512 against the published checksum -- fail closed.
	want, err := fetchText(base + ".sha512")
	if err != nil {
		return "", fmt.Errorf("fetch Kubo checksum: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(want))
	if len(fields) == 0 {
		return "", errors.New("empty Kubo checksum")
	}
	want = strings.ToLower(fields[0])
	got, err := sha512File(archive)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf("Kubo checksum mismatch -- refusing to install (want %s, got %s)", want, got)
	}
	logf("checksum verified")

	if err := extractIPFS(archive, filepath.Join(c.dataDir, "bin")); err != nil {
		return "", fmt.Errorf("extract Kubo: %w", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the fetched Kubo binary must be executable; 0700 is owner-only
		return "", err
	}
	return bin, nil
}

func latestKubo() (string, error) {
	body, err := fetchText("https://api.github.com/repos/ipfs/kubo/releases/latest")
	if err != nil {
		return "", err
	}
	var r struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return "", err
	}
	if r.TagName == "" {
		return "", errors.New("empty tag_name")
	}
	return r.TagName, nil
}

// ---- repo + daemon ----------------------------------------------------------

func ensureRepo(c cfg, ipfsBin string) error {
	if fileExists(filepath.Join(c.dataDir, "repo", "config")) {
		return nil
	}
	logf("initialising the IPFS repo (server profile)")
	if _, err := ipfsOut(c, ipfsBin, "init", "--profile=server"); err != nil {
		return err
	}
	return nil
}

// applyConfig runs every start so -gateway / -no-serve take effect even on an
// already-initialised repo.
func applyConfig(c cfg, ipfsBin string) {
	gw := ""
	if c.serve {
		gw = "/ip4/" + host(c.gateway) + "/tcp/" + port(c.gateway)
	}
	if out, err := ipfsOut(c, ipfsBin, "config", "Addresses.Gateway", gw); err != nil {
		logf("could not set the gateway address: %v (%s)", err, firstLine(out))
	}
	if out, err := ipfsOut(c, ipfsBin, "config", "--json", "Ipns.UsePubsub", "true"); err != nil {
		logf("could not enable IPNS pubsub: %v (%s)", err, firstLine(out))
	}
}

func startDaemon(ctx context.Context, c cfg, ipfsBin string) (*exec.Cmd, error) {
	cmd := exec.Command(ipfsBin, "daemon", "--enable-gc", "--migrate=true") // pubsub via Ipns.UsePubsub config
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repoPath(c))
	cmd.Stdout = prefixer("kubo")
	cmd.Stderr = prefixer("kubo")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func waitReady(c cfg, ipfsBin string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	apiFile := filepath.Join(repoPath(c), "api")
	for time.Now().Before(deadline) {
		// Probe only after the daemon has published its API file. Running an ipfs
		// command before then makes the CLI fall back to OFFLINE mode, which takes the
		// repo lock and kills the just-started daemon ("someone else has the lock").
		if fileExists(apiFile) {
			if _, err := ipfsOut(c, ipfsBin, "id", "-f=<id>"); err == nil {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return errors.New("timeout")
}

// ---- follow loop ------------------------------------------------------------

// followOnce resolves the FTB IPNS name and pins the current release if it moved.
// Returns the current CID and whether it changed from what was pinned.
func followOnce(c cfg, ipfsBin string) (string, bool) {
	cid, err := resolve(c, ipfsBin)
	if err != nil {
		logf("could not resolve the FTB release yet (%v) -- will retry", err)
		return "", false
	}
	old := readState(c)
	if cid == old {
		logf("up to date: %s", cid)
		return cid, false
	}
	logf("new release: %s (was %s)", cid, orNone(old))
	if _, err := ipfsOut(c, ipfsBin, "pin", "add", "/ipfs/"+cid); err != nil {
		logf("pin failed: %v -- will retry next cycle", err)
		return "", false
	}
	if old != "" && old != cid {
		if _, err := ipfsOut(c, ipfsBin, "pin", "rm", "/ipfs/"+old); err != nil {
			logf("could not unpin the old release %s: %v (it is garbage-collected anyway)", old, err)
		}
	}
	writeState(c, cid)
	logf("mirroring current release: %s", cid)
	return cid, true
}

func resolve(c cfg, ipfsBin string) (string, error) {
	out, err := ipfsOut(c, ipfsBin, "name", "resolve", "--nocache", "-r", "/ipns/"+c.ipns)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(firstLine(out)), "/ipfs/"), nil
}

// ---- small helpers ----------------------------------------------------------

func repoPath(c cfg) string { return filepath.Join(c.dataDir, "repo") }

func ipfsOut(c cfg, ipfsBin string, args ...string) (string, error) {
	cmd := exec.Command(ipfsBin, args...) // #nosec G204 -- operator tool: ipfsBin is the fetched Kubo binary, args are the tool's own constants, not remote input
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repoPath(c))
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func statePath(c cfg) string   { return filepath.Join(c.dataDir, "pinned-cid") }
func discordPath(c cfg) string { return filepath.Join(c.dataDir, "discord-id") }

func readState(c cfg) string {
	b, _ := os.ReadFile(statePath(c))
	return strings.TrimSpace(string(b))
}
func writeState(c cfg, cid string) {
	if err := os.WriteFile(statePath(c), []byte(cid+"\n"), 0o600); err != nil {
		logf("could not record the pinned CID: %v (re-pin tracking may repeat next cycle)", err)
	}
}

func loadDiscord(c cfg) string {
	b, _ := os.ReadFile(discordPath(c))
	return strings.TrimSpace(string(b))
}
func saveDiscord(c cfg, id string) error {
	return os.WriteFile(discordPath(c), []byte(strings.TrimSpace(id)+"\n"), 0o600) // #nosec G703 -- operator tool: path is under the operator's own -data dir
}

func validDiscord(id string) error {
	if id == "" {
		return errors.New("usage: ftb-node set-discord <your-discord-user-id> (a number like 123456789012345678)")
	}
	if len(id) < 15 || len(id) > 21 {
		return fmt.Errorf("that does not look like a Discord user ID (expected 15-21 digits, got %d chars)", len(id))
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return errors.New("a Discord user ID is all digits -- enable Developer Mode in Discord, right-click your name, Copy User ID")
		}
	}
	return nil
}

func banner(c cfg, id string) {
	fmt.Println()
	fmt.Println("  ------------------------------------------------------------")
	fmt.Println("   Free the Bird is now mirrored on this machine.")
	fmt.Printf("   Mirroring : /ipns/%s\n", c.ipns)
	fmt.Printf("   Pinned    : %s\n", orDash(readState(c)))
	if c.serve {
		fmt.Printf("   Open FTB  : http://%s/ipns/%s/\n", c.gateway, c.ipns)
	}
	fmt.Printf("   Node ID   : %s\n", id)
	if c.discordID != "" {
		fmt.Printf("   Discord   : %s  (contributor claim at /ipns/%s)\n", c.discordID, id)
	} else {
		fmt.Println("   Discord   : not set -- run `ftb-node set-discord <id>` to earn the contributor role")
	}
	fmt.Println("   It auto-follows new releases. Leave it running; Ctrl-C to stop.")
	fmt.Println("  ------------------------------------------------------------")
	fmt.Println()
}

func logf(f string, a ...any) { fmt.Printf("ftb-node: "+f+"\n", a...) }

func download(url, dst string) error {
	resp, err := httpGet(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	f, err := os.Create(dst) // #nosec G304 -- operator tool: dst is under the operator's own -data dir
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fetchText(url string) (string, error) {
	resp, err := httpGet(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

func httpGet(url string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "ftb-node/"+ftbNodeVersion)
	client := &http.Client{Timeout: 5 * time.Minute}
	return client.Do(req)
}

func sha512File(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- operator tool: path is the tool's own downloaded archive
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractIPFS(archivePath, binDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, binDir)
	}
	return extractTarGz(archivePath, binDir)
}

func extractTarGz(archivePath, binDir string) error {
	f, err := os.Open(archivePath) // #nosec G304 -- operator tool: archivePath is the tool's own downloaded archive
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) == binName() && h.Typeflag == tar.TypeReg {
			return writeFile(filepath.Join(binDir, binName()), tr)
		}
	}
	return errors.New("ipfs binary not found in archive")
}

func extractZip(archivePath, binDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) == binName() {
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeFile(filepath.Join(binDir, binName()), rc)
		}
	}
	return errors.New("ipfs binary not found in archive")
}

func writeFile(dst string, r io.Reader) error {
	out, err := os.Create(dst) // #nosec G304 -- operator tool: dst is under the operator's own -data dir
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "ipfs.exe"
	}
	return "ipfs"
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func try(s string, _ error) string { return s }

func orDash(s string) string {
	if s == "" {
		return "(pending first sync)"
	}
	return s
}
func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
func orUnset(s string) string {
	if s == "" {
		return "(not set -- run: ftb-node set-discord <id>)"
	}
	return s
}

func host(hp string) string {
	if i := strings.LastIndex(hp, ":"); i >= 0 {
		return hp[:i]
	}
	return hp
}
func port(hp string) string {
	if i := strings.LastIndex(hp, ":"); i >= 0 {
		return hp[i+1:]
	}
	return "8080"
}

// prefixer tags Kubo's own output so it is distinguishable in the console.
type linePrefixer struct {
	tag string
	buf []byte
}

func prefixer(tag string) io.Writer { return &linePrefixer{tag: tag} }

func (p *linePrefixer) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := strings.IndexByte(string(p.buf), '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(p.buf[:i]), "\r")
		if strings.TrimSpace(line) != "" {
			fmt.Printf("  [%s] %s\n", p.tag, line)
		}
		p.buf = p.buf[i+1:]
	}
	return len(b), nil
}
