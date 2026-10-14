# FTB build environment on a 1984 Hosting VPS (Iceland)

This stands up the environment for the immediate next step: compile every FTB contract and run
every test on a real toolchain. It is a plain Debian or Ubuntu VPS with Go and the neo-go compiler.
No Neo node is needed for this, because the contract tests run an in-memory chain.

What is automated versus what is yours. The bootstrap script `ops/ftb-env-setup.sh` does the whole
toolchain install and the build. You do the parts that need your account and your keys: create the
1984 account, order the VPS, and add your SSH key. I cannot and do not create the account, enter
payment, or log into the server for you.

## 1. Order the VPS

Create an account and order a VPS at 1984, and check the current price list, since third-party
figures disagree and some listings are stale: https://1984.hosting/product/pricelist/

Sizing for build and test: a Go compile plus the neotest suite is light but not tiny. A plan with
4 GB of RAM is comfortable; 2 GB is the practical minimum; 1 GB is likely too small for `go test`.
From third-party listings the tiers look like VPS 2 (1 CPU, 2 GB, 50 GB) around 10 USD per month and
VPS 3 (2 CPU, 4 GB, 80 GB) around 20 USD per month; confirm on the price list. VPS 3 is the
comfortable choice. You are billed monthly and there is a reported 7-day money-back window, so you
can tear it down after the build if you only need it to validate the code.

When ordering: choose a current Debian 12 or Ubuntu 24.04 (or 22.04) image, 64-bit, and add your SSH
public key so you get key-only login from the start. 1984 gives root SSH and a browser VNC console.

## 2. First login and basic hardening

This box will sit on the public internet, so lock it down before anything else. Replace `YOURUSER`
and the IP.

```bash
ssh root@SERVER_IP

# a non-root user with sudo
adduser YOURUSER
usermod -aG sudo YOURUSER
mkdir -p /home/YOURUSER/.ssh
cp ~/.ssh/authorized_keys /home/YOURUSER/.ssh/
chown -R YOURUSER:YOURUSER /home/YOURUSER/.ssh
chmod 700 /home/YOURUSER/.ssh && chmod 600 /home/YOURUSER/.ssh/authorized_keys

# key-only SSH: disable password and root login
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
systemctl restart ssh || systemctl restart sshd

# firewall: SSH only, plus automatic security updates
apt-get update -y && apt-get install -y ufw unattended-upgrades
ufw allow OpenSSH && ufw --force enable
dpkg-reconfigure -f noninteractive unattended-upgrades
```

Open a second terminal and confirm `ssh YOURUSER@SERVER_IP` works before you close the root session,
so a bad SSH edit cannot lock you out.

## 3. Get the FTB repo onto the server

From your own machine, as `YOURUSER`. Option A, copy the folder directly (works today, no git host
needed):

```bash
rsync -av --exclude build --exclude '*.wallet.json' --exclude 'web/config.js' \
  /path/to/ftb/ YOURUSER@SERVER_IP:~/ftb/
```

Option B, if the code is in a git repository, clone it on the server instead:

```bash
ssh YOURUSER@SERVER_IP
git clone <YOUR_GIT_URL> ~/ftb
```

## 4. Run the bootstrap

On the server, from inside the repo. Get the Go tarball checksum from https://go.dev/dl/ (find
`go1.26.0.linux-amd64.tar.gz`, copy its SHA256) and pass it so the download is verified:

```bash
ssh YOURUSER@SERVER_IP
cd ~/ftb
GO_SHA256=PASTE_THE_SHA256_HERE bash ops/ftb-env-setup.sh
```

It installs build tools and Go 1.26, installs the neo-go compiler, resolves modules, compiles every
contract into `build/`, and runs every test. A clean run ends with:

```
==> DONE: contracts compiled to /home/YOURUSER/ftb/build and all tests passed.
```

If a test fails on a neo-go version detail, the headers in the test files say what to adjust: the
`stackitem` shape for void methods, and the `InvokeAndCheck` helper used by the interest-accrual
test. To install the toolchain without building yet, run with `RUN_BUILD=no`.

## 5. What you have, and what comes next

You now have a reproducible environment that proves the code compiles and the tests pass, which is
the gate before everything else. `build/` holds the compiled `.nef` and `.manifest.json` for each
contract.

For later steps this same box is enough, with no extra services: deployment to TestNet uses a public
TestNet RPC with the `cmd/deploy` and `cmd/seed` tools (see `README.md`), so you do not need to run
your own Neo node. If you later want your own node for privacy or reliability, neo-go (already
installed) can run one, and you would open its ports in `ufw`; that is optional and not needed to
build, test, or deploy. The remaining project steps are in `DECISIONS.md` (the cap number and oracle
governance) and the pre-launch gate in `AUDIT_REPORT.md` (TestNet and bug bounty; the
external-audit leg is withdrawn per DECISIONS.md Decision 7).

## Notes

The server is in Iceland, so latency from far away is higher; for a build box that does not matter.
1984 is privacy-focused and accepts cards, PayPal, wire, and Bitcoin. Keep wallets and filled configs
off the server unless you need them there; `.gitignore` already excludes `*.wallet.json`,
`cmd/*/config.json`, and `web/config.js`, and the `rsync` above excludes them too.

Sources: [1984 price list](https://1984.hosting/product/pricelist/),
[1984 about](https://1984.hosting/about/),
[1984 review 2026 (WHTop)](https://www.whtop.com/amp/review/1984.hosting).
