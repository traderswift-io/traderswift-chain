# traderswift-1

The hub network: accounts and identity, charters, providers, vetoes, anchors, and the validator set. No balances, no token, no subnet data.

| | |
|---|---|
| Chain ID | `traderswift-1` |
| Binary | `traderswiftd`, built from this repository |
| Genesis | `genesis.json` and `genesis.sha256`, added here at the freeze (Fri 13 Nov 2026 08:00 UTC) |
| Genesis time | Fri 13 Nov 2026 11:13 UTC |
| Peers | `peers.txt`, one `nodeid@ip:26656` per line, added at genesis |
| Validators | four founding: `hub-1` and `hub-2` at `5000000`, `hub-3` and `hub-4` at `10000000`. Outside validators join after launch by `add-validator`, non-founding |

Not live yet: there is no genesis file in this folder until the freeze. To join once it is, follow [docs/VALIDATOR_JOIN.md](../../docs/VALIDATOR_JOIN.md).
