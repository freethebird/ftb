// SPDX-License-Identifier: MIT
pragma solidity 0.8.26;

// FtbHtlc - the Neo X (EVM) settlement leg of the non-custodial Neo N3 <-> Neo X atomic swap
// (Module 1, component 1). It mirrors the Neo N3 FtbHtlc semantics in EVM form: one swap per
// SHA-256 hashlock, claimed by the preimage or refunded after the timelock, with no owner,
// no oracle, no governance, no upgrade path, and no admin key.
//
// Built to CANONICAL_BIBLE_SOL.md. Dependency-free by Invariant: this contract imports
// nothing. The ERC-20 surface, the safe-transfer wrappers, and the reentrancy guard are
// inlined, so the deployed bytecode is a byte-deterministic function of this source (the
// artifact the IAR reviews and governance re-ratifies).
//
// One hash function across every chain: SHA-256, to match the Neo N3 contract's crypto.Sha256
// and Bitcoin's OP_SHA256.
//
// Weight and performance (Bible Book VII): custom errors (four-byte selectors, bound by
// selector in tests) rather than revert strings; the Swap record packs the one-byte state with
// the sender address into a single storage slot (five slots per swap, not six); claim and
// refund cache the record fields they use and emit before the payout (checks-effects-
// interactions). The explicit bound on the safe-transfer returndata copy against the
// returndata-bomb grief (Book VI.13) stays a reviewed-assembly refinement; the operative
// mitigation is the client asset allow-list, and the low-level calls here parse their return to
// a single bool. NOTES.md records the linter dispositions.
//
// The one safety rule this contract cannot enforce is the cross-leg timelock ladder
// (T_A > T_B); only the client sees both chains at once. See
// ../../../../docs/expansion/neox/HTLC_LINK_SPEC.md, section 5.
//
// Reference implementation. Internal audit lineage only -- verify before trusting value.

// Minimal ERC-20 surface this contract calls. Declared locally; not an imported dependency.
interface IERC20 {
    function transfer(address to, uint256 amount) external returns (bool);
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function balanceOf(address who) external view returns (uint256);
}

contract FtbHtlc {
    // The native coin (Neo X GAS) is addressed as the zero address; any other address is an
    // ERC-20 token.
    address internal constant NATIVE = address(0);

    uint256 public constant VERSION = 1;

    enum State {
        NONE, // 0: never used (the zero value, so an unused hashlock is distinguishable)
        LOCKED, // 1
        CLAIMED, // 2 (terminal)
        REFUNDED // 3 (terminal)
    }

    // Field order packs `state` (one byte) into the same 32-byte slot as `sender` (Book VII):
    // five slots per swap, not six.
    struct Swap {
        address sender;
        State state;
        address recipient;
        address token; // address(0) == native
        uint256 amount;
        uint256 timelock; // unix seconds (block.timestamp)
    }

    // One live swap per hashlock. The public getter is the swaps(hashlock) view.
    mapping(bytes32 => Swap) public swaps;

    // Storage reentrancy guard (1 = not entered, 2 = entered). Storage, not transient, for
    // compatibility across the Neo X EVM fork; the HTLC's gas cost makes the difference moot.
    uint256 private _entered = 1;

    // ---- custom errors (Book VII: weight; bound by selector in tests) ----
    error Reentrant();
    error AmountZero();
    error RecipientZero();
    error TimelockInPast();
    error HashlockUsed();
    error NativeAmountMismatch();
    error NativeNotExpected();
    error NoTokensReceived();
    error NotLocked();
    error PreimageMismatch();
    error TimelockNotReached();
    error TransferFailed();
    error TransferFromFailed();
    error NativeSendFailed();

    modifier nonReentrant() {
        if (_entered != 1) revert Reentrant();
        _entered = 2;
        _;
        _entered = 1;
    }

    event Locked(
        bytes32 indexed hashlock,
        address indexed sender,
        address indexed recipient,
        address token,
        uint256 amount,
        uint256 timelock
    );
    event Claimed(bytes32 indexed hashlock, bytes32 preimage);
    event Refunded(bytes32 indexed hashlock);

    // lock escrows `amount` of `token` (address(0) for native) under `hashlock`, payable to
    // `recipient`, refundable to the caller at/after `timelock`. For native, send exactly
    // `amount` as msg.value. For an ERC-20, the contract pulls via transferFrom and records the
    // ACTUAL received balance (fee-on-transfer safe). A reused hashlock reverts.
    function lock(bytes32 hashlock, address recipient, address token, uint256 amount, uint256 timelock)
        external
        payable
        nonReentrant
    {
        if (amount == 0) revert AmountZero();
        if (recipient == address(0)) revert RecipientZero();
        // Coarse timelock by design (Bible VI.15): no money pivots on a to-the-second deadline;
        // swap safety is the client ladder, not the chain clock.
        // forge-lint: disable-next-line(block-timestamp)
        if (timelock <= block.timestamp) revert TimelockInPast();
        if (swaps[hashlock].state != State.NONE) revert HashlockUsed();

        uint256 received;
        if (token == NATIVE) {
            if (msg.value != amount) revert NativeAmountMismatch();
            received = amount;
        } else {
            if (msg.value != 0) revert NativeNotExpected();
            uint256 pre = IERC20(token).balanceOf(address(this));
            _safeTransferFrom(token, msg.sender, address(this), amount);
            uint256 post = IERC20(token).balanceOf(address(this));
            received = post - pre; // actual delivered, not nominal
            if (received == 0) revert NoTokensReceived();
        }

        swaps[hashlock] = Swap({
            sender: msg.sender,
            state: State.LOCKED,
            recipient: recipient,
            token: token,
            amount: received,
            timelock: timelock
        });

        // The emit follows the measured-receipt pull; re-entry is blocked by nonReentrant, so
        // logs cannot be reordered or fabricated.
        // forge-lint: disable-next-line(reentrancy-events)
        emit Locked(hashlock, msg.sender, recipient, token, received, timelock);
    }

    // claim pays the recipient on a correct preimage. Permissionless: revealing the preimage is
    // the authorization, and funds only ever go to the recipient fixed at lock. The preimage is
    // emitted so the counterparty's watcher extracts it for the other leg.
    function claim(bytes32 hashlock, bytes32 preimage) external nonReentrant {
        Swap storage s = swaps[hashlock];
        if (s.state != State.LOCKED) revert NotLocked();
        if (sha256(abi.encodePacked(preimage)) != hashlock) revert PreimageMismatch();

        // Cache before the state write and the external call (CEI + Book VII caching).
        address recipient = s.recipient;
        address token = s.token;
        uint256 amount = s.amount;

        s.state = State.CLAIMED; // effect
        emit Claimed(hashlock, preimage); // effect (before the interaction)
        _payout(token, recipient, amount); // interaction, last
    }

    // refund returns the escrow to the sender at or after the timelock. Permissionless for the
    // same reason: funds only ever go to the sender fixed at lock.
    function refund(bytes32 hashlock) external nonReentrant {
        Swap storage s = swaps[hashlock];
        if (s.state != State.LOCKED) revert NotLocked();
        // Coarse timelock by design (Bible VI.15): see lock().
        // forge-lint: disable-next-line(block-timestamp)
        if (block.timestamp < s.timelock) revert TimelockNotReached();

        address sender = s.sender;
        address token = s.token;
        uint256 amount = s.amount;

        s.state = State.REFUNDED; // effect
        emit Refunded(hashlock); // effect (before the interaction)
        _payout(token, sender, amount); // interaction, last
    }

    // ---- views (parity with the N3 leg) ----

    function stateOf(bytes32 hashlock) external view returns (State) {
        return swaps[hashlock].state;
    }

    function exists(bytes32 hashlock) external view returns (bool) {
        return swaps[hashlock].state != State.NONE;
    }

    // ---- internals ----

    function _payout(address token, address to, uint256 amount) private {
        if (token == NATIVE) {
            // nonReentrant holds across this call, so re-entry is blocked; the destination is
            // the address fixed at lock (the HTLC guarantee, Bible Book IV rule 13), so the
            // user-controlled destination is the intended one, not an open sink.
            // forge-lint: disable-next-line(reentrancy-eth, arbitrary-send-eth)
            (bool ok,) = payable(to).call{value: amount}("");
            if (!ok) revert NativeSendFailed();
        } else {
            _safeTransfer(token, to, amount);
        }
    }

    // Tolerates non-standard ERC-20s: success plus, if any data was returned, a true boolean.
    // No assumption that transfer returns true or moves the full amount. nonReentrant guards
    // every entry, so the low-level call carries no reentrancy risk the guard does not cover.
    function _safeTransfer(address token, address to, uint256 amount) private {
        // forge-lint: disable-next-line(reentrancy-no-eth)
        (bool ok, bytes memory data) = token.call(abi.encodeCall(IERC20.transfer, (to, amount)));
        if (!ok || (data.length != 0 && !abi.decode(data, (bool)))) revert TransferFailed();
    }

    function _safeTransferFrom(address token, address from, address to, uint256 amount) private {
        // The pull precedes the swaps write only for measured receipt (Bible Book II point 9);
        // nonReentrant blocks any re-entry through the called token.
        // forge-lint: disable-next-line(reentrancy-no-eth)
        (bool ok, bytes memory data) = token.call(abi.encodeCall(IERC20.transferFrom, (from, to, amount)));
        if (!ok || (data.length != 0 && !abi.decode(data, (bool)))) revert TransferFromFailed();
    }
}
