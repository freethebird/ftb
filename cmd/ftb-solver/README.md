# ftb-solver

Reference FASP solver. It wires the Neo N3 and Neo X HTLC adapters
(pkg/chain/neon3, pkg/chain/neox) behind the fasp.HTLC interface; the maker/taker
orchestration and its safety checks (the timelock ladder, VerifyLock) live in
pkg/fasp. No new dependency beyond neo-go: see pkg/evm/NOTES.md.

## status (read path, no keys)

    ftb-solver status -chain neox -rpc https://testnet.rpc.banelabs.org/ \
      -contract 0x607FBFC1E22Dcbe152D6e8A7ffD7902f0DD7da85 \
      -hashlock 632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80

    ftb-solver status -chain n3 -rpc https://testnet1.neo.coz.io:443 \
      -contract 0x3d7e6916972f550ba3405204d3f3bf328996d656 \
      -hashlock 632625cf8d1a67e550dba228cd567bf3a90ee7130db342012c78ddabb62dfb80

Prints the on-chain swap record and, if the secret has been revealed, the
preimage read from the Claimed event. Executing a swap leg (Lock/Claim/Refund)
requires a configured wallet on that chain.

## run (write path, needs keys)

Drives a full atomic swap across both legs. Copy the template, fill in
RPC/contract/amounts and the signing material, then run:

    cp config.example.json config.json      # config.json is gitignored
    # edit config.json: legB.key (secp256k1 hex), legA.wallet + walletPass + account
    ftb-solver run -config config.json

The driver generates a secret (or takes -secret <hex>), validates the timelock
ladder (fasp.ComputeTimelocks over the two chain profiles), derives the absolute
timelocks (N3 by block height, Neo X by unix time), builds the fasp.Swap, and runs
the sequence with phase logging:

    1/4 solver locks B
    2/4 user verifies B and locks A    (VerifyLock: recipient/amount/token/timelock)
    3/4 user claims B (reveals secret)
    4/4 solver reads secret and claims A

On failure it prints refund guidance (SolverRefundB after T_B, UserRefundA after
T_A). legA is the user's leg (longer timelock), legB the solver's leg (shorter).
The config holds secrets - keep config.json out of version control.
