// SPDX-License-Identifier: MIT
pragma solidity 0.8.26;

// Neo X side of the cross-chain differential KAT. Runs the shared vectors (KatVectors.sol,
// generated with kat/vectors.json by kat/gen.py) through FtbHtlc.sol and asserts the same
// lock/claim/refund outcomes the N3 neotest harness (ftb_htlc_kat_test.go) asserts. The hash
// pin (sha256(preimage) == hashlock) proves the SHA-256 commitment is identical across chains.
// Dependency-free: inline cheatcode interface and mock token, no forge-std.

import {FtbHtlc} from "../src/FtbHtlc.sol";
import {KatVectors} from "./KatVectors.sol";

interface IVmKat {
    function prank(address) external;
    function warp(uint256) external;
    function expectRevert(bytes4) external;
}

contract KatToken {
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

contract FtbHtlcKatTest {
    IVmKat internal constant vm = IVmKat(0x7109709ECfa91a80626fF3989D68f67F5b1DD12D);
    address internal constant alice = address(0xA11CE);
    address internal constant bob = address(0xB0B);
    address internal constant other = address(0x0E);
    uint256 internal constant WINDOW = 100;

    function test_KatDifferential() public {
        for (uint256 i = 0; i < KatVectors.COUNT; i++) {
            (uint256 amount, bytes32 hashlock, bytes32 preimage, bool isClaim) = KatVectors.get(i);

            // Hash pin: the on-chain SHA-256 matches the shared vector, identical to the N3 leg.
            require(sha256(abi.encodePacked(preimage)) == hashlock, "hash pin");

            FtbHtlc htlc = new FtbHtlc();
            KatToken tok = new KatToken();
            tok.mint(alice, amount);
            vm.prank(alice);
            tok.approve(address(htlc), amount);
            vm.prank(alice);
            htlc.lock(hashlock, bob, address(tok), amount, block.timestamp + WINDOW);
            require(htlc.stateOf(hashlock) == FtbHtlc.State.LOCKED, "locked");

            if (isClaim) {
                vm.prank(other);
                vm.expectRevert(FtbHtlc.PreimageMismatch.selector);
                htlc.claim(hashlock, KatVectors.WRONG_PREIMAGE);

                vm.prank(other);
                htlc.claim(hashlock, preimage);
                require(htlc.stateOf(hashlock) == FtbHtlc.State.CLAIMED, "claimed");
                require(tok.balanceOf(bob) == amount, "bob paid");
                require(tok.balanceOf(address(htlc)) == 0, "htlc empty");

                vm.prank(other);
                vm.expectRevert(FtbHtlc.NotLocked.selector);
                htlc.claim(hashlock, preimage);
            } else {
                vm.prank(other);
                vm.expectRevert(FtbHtlc.TimelockNotReached.selector);
                htlc.refund(hashlock);

                vm.warp(block.timestamp + WINDOW + 1);
                vm.prank(other);
                htlc.refund(hashlock);
                require(htlc.stateOf(hashlock) == FtbHtlc.State.REFUNDED, "refunded");
                require(tok.balanceOf(alice) == amount, "alice refunded");
                require(tok.balanceOf(address(htlc)) == 0, "htlc empty");
            }
        }
    }
}
