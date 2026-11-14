# traderswift-1

The hub network: accounts and identity, charters, providers, vetoes, anchors, and the validator set. No balances, no token, no subnet data.

| | |
|---|---|
| Chain ID | `traderswift-1` |
| Binary | `traderswiftd`, built from this repository |
| Genesis | `genesis.json` and `genesis.sha256`, added here at the freeze (Sat 14 Nov 2026) |
| Genesis time | Sat 14 Nov 2026 14:13 UTC |
| Peers | `peers.txt`, one `nodeid@ip:26656` per line, added at genesis |
| Validators | four founding, `hub-1` to `hub-4`, `10000000` each. Outside validators join after launch by `add-validator`, non-founding |

Live: `genesis.json`, `genesis.sha256` and `peers.txt` are in this folder. To join, follow [docs/VALIDATOR_JOIN.md](../../docs/VALIDATOR_JOIN.md).
