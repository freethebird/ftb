# FTB node: operator runbook (Neo N3 TestNet)

Each community VM runs a **neo-go full node** on Neo N3 TestNet with a public, TLS-wrapped,
CORS-enabled JSON-RPC endpoint. The FTB front end reads from these endpoints with failover -
no server-side code, no keys, nothing custodial. This is what makes the DEX front end
decentralized: anyone's node can serve anyone's reads.

Time: ~30 minutes. Cost: a small VM (2 vCPU / 4 GB RAM / 40 GB disk is comfortable).

## What you end with

`https://<your-domain>/` → Caddy (TLS, automatic) → neo-go RPC on 127.0.0.1:10332

## Steps

1. DNS: point an A record (for example `node1.ftb.example`) at your VM.
2. Run the installer as root or with sudo:

   ```sh
   curl -L https://raw.githubusercontent.com/<repo>/main/ftb/node/setup.sh | sudo sh
   ```

   or, from a checkout: `sudo sh node/setup.sh`

   It installs the pinned neo-go release, writes `protocol.testnet.yml`, registers and
   starts the `ftb-node` systemd unit, and installs Caddy.
3. Put your domain into `/etc/caddy/Caddyfile` (the installer writes a template with
   `node.example.invalid` - replace it), then `sudo systemctl reload caddy`.
4. Verify:

   ```sh
   curl -s -X POST https://<your-domain>/ -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"getblockcount","params":[]}'
   ```

   A number means healthy. Initial sync takes a while; the endpoint answers during sync.
5. Hand your URL to whoever maintains `web/config.js` - it goes at the top of the `rpcs`
   list, ahead of the public fallbacks.

## Rules of the road

- The node holds **no wallet and no keys**. RPC is read-only by policy: do not enable
  `SessionPool`/unlocking, do not open the P2P port to anything but the network.
- TLS is not optional: browsers refuse http calls from an https page (mixed content).
- One operator, one node, one domain. Five operators → five independent endpoints.
- Updates: `sudo systemctl restart ftb-node` after replacing the binary.
