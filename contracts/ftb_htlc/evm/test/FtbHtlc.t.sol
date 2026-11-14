// SPDX-License-Identifier: MIT
pragma solidity 0.8.26;

// Dependency-free Foundry suite for FtbHtlc (CANONICAL_BIBLE_SOL.md: strongest state is the
// empty dev graph - no forge-std). The cheatcode interface is declared inline and assertions
// are require-based, so `forge test` runs with nothing installed but Foundry itself.
//
// Covers the Bible gate rows: lifecycle (ERC-20 + native), wrong preimage, double-claim, refund
// timing, reused hashlock, validation, the token-quirk trio (fee-on-transfer measured receipt,
// no-return, false-return), reentrancy (malicious recipient on native, malicious token
// cross-function), and fuzzed conservation. Negative paths bind the custom-error selectors.
//
// Run: forge test  (from contracts/ftb_htlc/evm/)

import {FtbHtlc, IERC20} from "../src/FtbHtlc.sol";

// Minimal Foundry cheatcode surface (the VM at the well-known address).
interface IVm {
    function warp(uint256) external;
    function prank(address) external;
    function deal(address, uint256) external;
    function expectRevert(bytes4) external;
}

// ---- mock tokens ----

contract GoodToken {
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    function mint(address to, uint256 a) external {
        balanceOf[to] += a;
    }

    function approve(address s, uint256 a) external returns (bool) {
        allowance[msg.sender][s] = a;
        return true;
    }

    function transfer(address to, uint256 a) external returns (bool) {
        balanceOf[msg.sender] -= a;
        balanceOf[to] += a;
        return true;
    }

    function transferFrom(address f, address t, uint256 a) external returns (bool) {
        uint256 al = allowance[f][msg.sender];
        require(al >= a, "allowance");
        if (al != type(uint256).max) allowance[f][msg.sender] = al - a;
        balanceOf[f] -= a;
        balanceOf[t] += a;
        return true;
    }
}

// 10% fee burned on every move: tests measured-receipt accounting.
contract FeeToken {
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    function mint(address to, uint256 a) external {
        balanceOf[to] += a;
    }

    function approve(address s, uint256 a) external returns (bool) {
        allowance[msg.sender][s] = a;
        return true;
    }

    function _move(address f, address t, uint256 a) internal {
        uint256 fee = a / 10;
        balanceOf[f] -= a;
        balanceOf[t] += a - fee;
    }

    function transfer(address to, uint256 a) external returns (bool) {
        _move(msg.sender, to, a);
        return true;
    }

    function transferFrom(address f, address t, uint256 a) external returns (bool) {
        uint256 al = allowance[f][msg.sender];
        require(al >= a, "allowance");
        if (al != type(uint256).max) allowance[f][msg.sender] = al - a;
        _move(f, t, a);
        return true;
    }
}

// transfer / transferFrom return no data: tests the no-return-value path (USDT/BNB class).
contract NoReturnToken {
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    function mint(address to, uint256 a) external {
        balanceOf[to] += a;
    }

    function approve(address s, uint256 a) external {
        allowance[msg.sender][s] = a;
    }

    function transfer(address to, uint256 a) external {
        balanceOf[msg.sender] -= a;
        balanceOf[to] += a;
    }

    function transferFrom(address f, address t, uint256 a) external {
        uint256 al = allowance[f][msg.sender];
        require(al >= a, "allowance");
        if (al != type(uint256).max) allowance[f][msg.sender] = al - a;
        balanceOf[f] -= a;
        balanceOf[t] += a;
    }
}

// transferFrom returns false: the safe-transfer must reject it.
contract FalseToken {
    function balanceOf(address) external pure returns (uint256) {
        return 0;
    }

    function approve(address, uint256) external pure returns (bool) {
        return true;
    }

    function transfer(address, uint256) external pure returns (bool) {
        return false;
    }

    function transferFrom(address, address, uint256) external pure returns (bool) {
        return false;
    }
}

// Re-enters claim on receiving native; swallows the guard revert. Proves the guard stops a
// double-payout on the native path.
contract ReentrantRecipient {
    FtbHtlc internal htlc;
    bytes32 internal hl;
    bytes32 internal pre;
    bool public reentered;

    constructor(FtbHtlc h) {
        htlc = h;
    }

    function arm(bytes32 _hl, bytes32 _pre) external {
        hl = _hl;
        pre = _pre;
    }

    receive() external payable {
        try htlc.claim(hl, pre) {
            reentered = true;
        } catch {}
    }
}

// Re-enters claim on an already-locked swap during its own transferFrom. Proves cross-function
// reentrancy is blocked: the nested claim reverts under the guard and bubbles, so lock fails.
contract ReentrantToken {
    FtbHtlc internal htlc;
    bytes32 internal hl;
    bytes32 internal pre;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    constructor(FtbHtlc h) {
        htlc = h;
    }

    function arm(bytes32 _hl, bytes32 _pre) external {
        hl = _hl;
        pre = _pre;
    }

    function mint(address to, uint256 a) external {
        balanceOf[to] += a;
    }

    function approve(address s, uint256 a) external returns (bool) {
        allowance[msg.sender][s] = a;
        return true;
    }

    function transfer(address to, uint256 a) external returns (bool) {
        balanceOf[msg.sender] -= a;
        balanceOf[to] += a;
        return true;
    }

    function transferFrom(address f, address t, uint256 a) external returns (bool) {
        htlc.claim(hl, pre); // re-entry: reverts under the guard and bubbles
        uint256 al = allowance[f][msg.sender];
        if (al != type(uint256).max) allowance[f][msg.sender] = al - a;
        balanceOf[f] -= a;
        balanceOf[t] += a;
        return true;
    }
}

// ---- the suite ----

contract FtbHtlcTest {
    IVm internal constant vm = IVm(0x7109709ECfa91a80626fF3989D68f67F5b1DD12D);
    address internal constant NATIVE = address(0);

    FtbHtlc internal htlc;
    GoodToken internal tok;

    address internal alice = address(0xA11CE);
    address internal bob = address(0xB0B);
    address internal other = address(0x0E);
    address internal solver = address(0x5017E5);

    function setUp() public {
        htlc = new FtbHtlc();
        tok = new GoodToken();
    }

    // ---- helpers ----

    function _mk(string memory tag) internal pure returns (bytes32 pre, bytes32 hl) {
        pre = keccak256(abi.encodePacked(tag));
        hl = sha256(abi.encodePacked(pre));
    }

    function _eqU(uint256 a, uint256 b, string memory m) internal pure {
        require(a == b, m);
    }

    function _true(bool c, string memory m) internal pure {
        require(c, m);
    }

    function _state(bytes32 hl, FtbHtlc.State want, string memory m) internal view {
        _true(htlc.stateOf(hl) == want, m);
    }

    function _lockTok(address who, address token, uint256 amount, bytes32 hl, address recipient, uint256 tl) internal {
        vm.prank(who);
        GoodToken(token).approve(address(htlc), amount);
        vm.prank(who);
        htlc.lock(hl, recipient, token, amount, tl);
    }

    // ---- lifecycle ----

    function test_LockClaimERC20() public {
        (bytes32 pre, bytes32 hl) = _mk("a");
        tok.mint(alice, 1000);
        _lockTok(alice, address(tok), 1000, hl, bob, block.timestamp + 100);
        _state(hl, FtbHtlc.State.LOCKED, "locked");
        _eqU(tok.balanceOf(address(htlc)), 1000, "escrowed");
        _eqU(tok.balanceOf(alice), 0, "alice drained");

        vm.prank(other); // permissionless
        htlc.claim(hl, pre);
        _state(hl, FtbHtlc.State.CLAIMED, "claimed");
        _eqU(tok.balanceOf(bob), 1000, "bob paid");
        _eqU(tok.balanceOf(address(htlc)), 0, "htlc empty");
    }

    function test_LockClaimNative() public {
        (bytes32 pre, bytes32 hl) = _mk("n");
        vm.deal(alice, 1 ether);
        vm.prank(alice);
        htlc.lock{value: 1 ether}(hl, bob, NATIVE, 1 ether, block.timestamp + 100);
        _state(hl, FtbHtlc.State.LOCKED, "locked");
        _eqU(address(htlc).balance, 1 ether, "escrowed native");

        vm.prank(other);
        htlc.claim(hl, pre);
        _eqU(bob.balance, 1 ether, "bob paid native");
        _eqU(address(htlc).balance, 0, "htlc empty");
    }

    function test_WrongPreimage() public {
        (, bytes32 hl) = _mk("b");
        tok.mint(alice, 1000);
        _lockTok(alice, address(tok), 1000, hl, bob, block.timestamp + 100);
        (bytes32 wrong,) = _mk("not-b");
        vm.prank(other);
        vm.expectRevert(FtbHtlc.PreimageMismatch.selector);
        htlc.claim(hl, wrong);
        _state(hl, FtbHtlc.State.LOCKED, "still locked");
    }

    function test_DoubleClaim() public {
        (bytes32 pre, bytes32 hl) = _mk("c");
        tok.mint(alice, 1000);
        _lockTok(alice, address(tok), 1000, hl, bob, block.timestamp + 100);
        vm.prank(other);
        htlc.claim(hl, pre);
        vm.prank(other);
        vm.expectRevert(FtbHtlc.NotLocked.selector);
        htlc.claim(hl, pre);
        _eqU(tok.balanceOf(bob), 1000, "paid once");
    }

    function test_RefundBeforeTimelock() public {
        (, bytes32 hl) = _mk("d");
        tok.mint(alice, 1000);
        _lockTok(alice, address(tok), 1000, hl, bob, block.timestamp + 100);
        vm.prank(other);
        vm.expectRevert(FtbHtlc.TimelockNotReached.selector);
        htlc.refund(hl);
    }

    function test_RefundAfterTimelock() public {
        (, bytes32 hl) = _mk("e");
        tok.mint(alice, 1000);
        uint256 tl = block.timestamp + 100;
        _lockTok(alice, address(tok), 1000, hl, bob, tl);
        vm.warp(tl);
        vm.prank(other);
        htlc.refund(hl);
        _state(hl, FtbHtlc.State.REFUNDED, "refunded");
        _eqU(tok.balanceOf(alice), 1000, "alice refunded");
        _eqU(tok.balanceOf(address(htlc)), 0, "htlc empty");
    }

    function test_ReusedHashlock() public {
        (, bytes32 hl) = _mk("f");
        tok.mint(alice, 1000);
        _lockTok(alice, address(tok), 500, hl, bob, block.timestamp + 100);
        vm.prank(alice);
        tok.approve(address(htlc), 500);
        vm.prank(alice);
        vm.expectRevert(FtbHtlc.HashlockUsed.selector);
        htlc.lock(hl, bob, address(tok), 500, block.timestamp + 100);
    }

    function test_Validation() public {
        (, bytes32 hl) = _mk("g");
        vm.deal(alice, 10 ether);

        vm.prank(alice);
        vm.expectRevert(FtbHtlc.AmountZero.selector);
        htlc.lock(hl, bob, NATIVE, 0, block.timestamp + 100);

        vm.prank(alice);
        vm.expectRevert(FtbHtlc.RecipientZero.selector);
        htlc.lock{value: 1}(hl, address(0), NATIVE, 1, block.timestamp + 100);

        vm.prank(alice);
        vm.expectRevert(FtbHtlc.TimelockInPast.selector);
        htlc.lock{value: 1}(hl, bob, NATIVE, 1, block.timestamp);

        vm.prank(alice);
        vm.expectRevert(FtbHtlc.NativeAmountMismatch.selector);
        htlc.lock{value: 1}(hl, bob, NATIVE, 2, block.timestamp + 100);

        tok.mint(alice, 100);
        vm.prank(alice);
        tok.approve(address(htlc), 100);
        vm.prank(alice);
        vm.expectRevert(FtbHtlc.NativeNotExpected.selector);
        htlc.lock{value: 1}(hl, bob, address(tok), 100, block.timestamp + 100);

        _state(hl, FtbHtlc.State.NONE, "nothing persisted");
    }

    // ---- token-quirk gates (Book VI.8) ----

    function test_FeeOnTransfer() public {
        (bytes32 pre, bytes32 hl) = _mk("fee");
        FeeToken fee = new FeeToken();
        fee.mint(alice, 1000);
        vm.prank(alice);
        fee.approve(address(htlc), 1000);
        vm.prank(alice);
        htlc.lock(hl, bob, address(fee), 1000, block.timestamp + 100);
        // 10% skimmed on the pull: the escrow records the actual 900 delivered, not 1000.
        (,,,, uint256 amount,) = htlc.swaps(hl);
        _eqU(amount, 900, "measured receipt 900");
        _eqU(fee.balanceOf(address(htlc)), 900, "htlc holds 900");

        vm.prank(other);
        htlc.claim(hl, pre);
        // 10% skimmed again on the payout: bob gets 810. The contract never over-pays.
        _eqU(fee.balanceOf(bob), 810, "bob gets 810");
        _eqU(fee.balanceOf(address(htlc)), 0, "htlc empty");
    }

    function test_NoReturnToken() public {
        (bytes32 pre, bytes32 hl) = _mk("nr");
        NoReturnToken nr = new NoReturnToken();
        nr.mint(alice, 1000);
        vm.prank(alice);
        nr.approve(address(htlc), 1000);
        vm.prank(alice);
        htlc.lock(hl, bob, address(nr), 1000, block.timestamp + 100);
        _eqU(nr.balanceOf(address(htlc)), 1000, "escrowed");
        vm.prank(other);
        htlc.claim(hl, pre);
        _eqU(nr.balanceOf(bob), 1000, "bob paid");
    }

    function test_FalseReturnTokenRejected() public {
        (, bytes32 hl) = _mk("false");
        FalseToken f = new FalseToken();
        vm.prank(alice);
        vm.expectRevert(FtbHtlc.TransferFromFailed.selector);
        htlc.lock(hl, bob, address(f), 1000, block.timestamp + 100);
    }

    // ---- reentrancy gates (Book VI.1) ----

    function test_ReentrantRecipientNativeBlocked() public {
        (bytes32 pre, bytes32 hl) = _mk("rr");
        ReentrantRecipient rr = new ReentrantRecipient(htlc);
        rr.arm(hl, pre);
        vm.deal(alice, 1 ether);
        vm.prank(alice);
        htlc.lock{value: 1 ether}(hl, address(rr), NATIVE, 1 ether, block.timestamp + 100);

        vm.prank(other);
        htlc.claim(hl, pre);
        _true(!rr.reentered(), "re-entry was blocked");
        _eqU(address(rr).balance, 1 ether, "paid exactly once");
        _state(hl, FtbHtlc.State.CLAIMED, "claimed once");
    }

    function test_ReentrantTokenCrossFunctionBlocked() public {
        (bytes32 preA, bytes32 hlA) = _mk("legit");
        tok.mint(alice, 1000);
        _lockTok(alice, address(tok), 1000, hlA, bob, block.timestamp + 100);

        ReentrantToken rt = new ReentrantToken(htlc);
        rt.arm(hlA, preA);
        rt.mint(alice, 100);
        vm.prank(alice);
        rt.approve(address(htlc), 100);

        (, bytes32 hlB) = _mk("evil");
        vm.prank(alice);
        vm.expectRevert(FtbHtlc.TransferFromFailed.selector);
        htlc.lock(hlB, bob, address(rt), 100, block.timestamp + 100);

        // The legit swap was never drained by the re-entry.
        _state(hlA, FtbHtlc.State.LOCKED, "legit still locked");
        _eqU(tok.balanceOf(bob), 0, "no cross-function drain");
    }

    function test_SettleUnknown() public {
        (bytes32 pre, bytes32 hl) = _mk("unknown");
        vm.prank(other);
        vm.expectRevert(FtbHtlc.NotLocked.selector);
        htlc.claim(hl, pre);
        vm.prank(other);
        vm.expectRevert(FtbHtlc.NotLocked.selector);
        htlc.refund(hl);
    }

    // ---- fuzzed conservation (Book II point 12 / PROVE direction) ----

    function testFuzz_LockClaimConservation(uint96 amount, bytes32 secret) public {
        if (amount == 0) return;
        bytes32 hl = sha256(abi.encodePacked(secret));
        tok.mint(alice, amount);
        vm.prank(alice);
        tok.approve(address(htlc), amount);
        vm.prank(alice);
        htlc.lock(hl, bob, address(tok), amount, block.timestamp + 100);
        _eqU(tok.balanceOf(address(htlc)), amount, "escrowed");
        vm.prank(other);
        htlc.claim(hl, secret);
        _eqU(tok.balanceOf(bob), amount, "bob paid in full");
        _eqU(tok.balanceOf(address(htlc)), 0, "nothing left");
    }

    function testFuzz_LockRefundConservation(uint96 amount, bytes32 secret) public {
        if (amount == 0) return;
        bytes32 hl = sha256(abi.encodePacked(secret));
        tok.mint(alice, amount);
        uint256 tl = block.timestamp + 100;
        vm.prank(alice);
        tok.approve(address(htlc), amount);
        vm.prank(alice);
        htlc.lock(hl, bob, address(tok), amount, tl);
        vm.warp(tl);
        vm.prank(other);
        htlc.refund(hl);
        _eqU(tok.balanceOf(alice), amount, "alice refunded in full");
        _eqU(tok.balanceOf(address(htlc)), 0, "nothing left");
    }
}
