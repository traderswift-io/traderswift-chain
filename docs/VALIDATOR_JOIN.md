# Running a traderswift-1 validator

This page is for people who want to run a node that helps keep the hub chain going. You don't need any of it to understand or use TraderSwift.

**Just want to see it? Start here:** [what the hub chain is](https://github.com/traderswift-io/traderswift-chain#traderswift-chain), then [the live network files](https://github.com/traderswift-io/traderswift-chain/tree/main/networks/traderswift-1): its chain ID, genesis and peers, readable in the browser. No install, no keys, no sign-up.

For an outside operator. You run one node that produces blocks on `traderswift-1`, the hub chain every subnet anchors onto. Everything you need is in this repository. You can clone, configure, start and sync without contacting us. The only step that involves us is the last one, adding you to the validator set (section 7).

A validator and a provider are different things. A validator produces blocks and needs us once (section 7). A provider (a timeseries provider, say) offers a subnet's service and anchors its hashes; it joins with no contact at all (section 11).

The network files are in [`networks/traderswift-1/`](../networks/traderswift-1/): chain ID, `genesis.json`, its SHA-256 and the peer list.

## What you get, and what you never get

| You get | You never get |
|---|---|
| A vote in block production, in proportion to the power the admin gives you | Any subnet's data, keys or deploy kit |
| A full copy of the chain: accounts, charters, providers, vetoes, anchors | The power to set charters, veto, or change the validator set |
| Your node's own keys, made and kept on your machine | Any secret of ours, or any of our machines' keys |

Your node checks and orders transactions. The chain holds identities and hashes only: no balances, no token, no private content.

**The one-third rule.** While outside validators are few, the chain refuses any set where a single non-founding validator holds one third of the voting power or more. The founders cannot be outvoted by one outsider, and one outsider cannot halt the chain alone.

## 1. Machine

- Linux x86_64, 1 vCPU, 2 GB RAM, 20 GB disk is enough today. The binary is about 108 MB; a node used about 90 MB of memory after start.
- A public IP, fixed if you can. Our nodes accept p2p on 26656 from anywhere.
- A clock kept by NTP.
- No other service on the machine needs our trust. Keep it to yourself.

## 2. The binary

Either build it from source:

```
git clone https://github.com/traderswift-io/traderswift-chain.git
cd traderswift-chain
go build -trimpath -o traderswiftd .     # Go 1.26.5, as in go.mod
sha256sum traderswiftd
```

There is no published binary yet, so build from source.

Install it as `/opt/traderswift/bin/traderswiftd` and run the node as its own user:

```
sudo useradd --system --home /var/lib/traderswift --create-home traderswift
sudo install -d -m 0755 /opt/traderswift/bin
sudo install -m 0755 traderswiftd /opt/traderswift/bin/traderswiftd
```

## 3. Your keys, made on your machine

```
sudo -u traderswift /opt/traderswift/bin/traderswiftd init <your moniker> --chain-id traderswift-1 --home /var/lib/traderswift/chain
```

This writes two keys you keep:

| File | What | If lost or copied |
|---|---|---|
| `config/node_key.json` | p2p identity (your node ID) | Make a new one |
| `config/priv_validator_key.json` | consensus key that signs blocks | Copied: open an issue at once and we remove it. Never run two nodes with it: both would sign and the chain sees double signing |

Back up `priv_validator_key.json` offline. Never send it to anyone, us included.

You send us nothing yet. Section 7 says what to send once you are synced.

### When you join

Genesis is Sat 14 Nov 2026 14:13 UTC, with the TraderSwift hub validators only. Every outside validator joins after launch the normal way: run sections 1 to 6, sync, then section 7. We add you with `add-validator` (not a founding validator, power under one third of the total).

## 4. Genesis and peers

From the clone, check the hash, then copy genesis in:

```
cd traderswift-chain/networks/traderswift-1
sha256sum -c genesis.sha256                     # must print: genesis.json: OK
sudo -u traderswift cp genesis.json /var/lib/traderswift/chain/config/genesis.json
paste -sd, peers.txt                            # your persistent_peers value
```

In `config/config.toml`:

```
[p2p]
laddr = "tcp://0.0.0.0:26656"
external_address = "tcp://<your public ip>:26656"
persistent_peers = "<the output of paste -sd, peers.txt>"
pex = false

[rpc]
laddr = "tcp://127.0.0.1:26657"
```

In `config/app.toml`: `minimum-gas-prices = "0nofee"`. Keep `[api]` and `[grpc]` on `127.0.0.1`.

## 5. Sync

**Block sync (default).** Start the node; it replays every block from genesis. The chain is small, so this is minutes.

**State sync (optional, not offered yet).** Only once we publish a read-only RPC address in `networks/traderswift-1/`. In `config.toml`:

```
[statesync]
enable = true
rpc_servers = "<rpc 1>,<rpc 2>"
trust_height = <height we publish>
trust_hash = "<block hash at that height>"
trust_period = "168h0m0s"
```

Check `trust_hash` yourself against a second source before you start. State sync needs our nodes to take snapshots (`state-sync.snapshot-interval` in their `app.toml`), which we turn on before we offer it.

## 6. Run under systemd

`/etc/systemd/system/traderswift-chain.service`:

```
[Unit]
Description=traderswift-1 hub node
After=network-online.target
Wants=network-online.target

[Service]
User=traderswift
ExecStart=/opt/traderswift/bin/traderswiftd start --home /var/lib/traderswift/chain
Restart=on-failure
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

```
sudo systemctl daemon-reload && sudo systemctl enable --now traderswift-chain.service
curl -s http://127.0.0.1:26657/status | jq .result.sync_info
```

Wait until `catching_up` is `false`, and compare your height with a founding node's if you can. Until we add you, you are a full node: you follow the chain and sign nothing.

## 7. We add you

When you are synced, open **New issue, then Validator key** in this repository (title `validator: <your moniker>`). It asks for the output of:

```
sudo -u traderswift /opt/traderswift/bin/traderswiftd comet show-validator --home /var/lib/traderswift/chain   # consensus public key
sudo -u traderswift /opt/traderswift/bin/traderswiftd comet show-node-id --home /var/lib/traderswift/chain     # node ID
curl -s http://127.0.0.1:26657/status | jq -r .result.sync_info.latest_block_height           # your height
```

plus your public IP. These are public keys and IDs only. Nothing here lets anyone act as you. Never paste `priv_validator_key.json` or `node_key.json`.

Our admin then sends one transaction. The admin is one address fixed in genesis: a 2-of-3 multisig held by the founders. The command below is one of its signatures; the multisig steps are `tx sign` and `tx multi-sign`:

```
traderswiftd tx validatorset add-validator \
  --consensus-public-key <the "key" value from show-validator> \
  --power <power> --moniker <your moniker> \
  --from <admin> --chain-id traderswift-1
```

`--founding` is never set for an outside validator. The chain refuses the add if it would give you one third of the power or more. The change reaches CometBFT at the end of that block and takes effect two blocks later. You see it as your address in `/validators`:

```
curl -s http://127.0.0.1:26657/validators | jq '.result.validators[] | {address, voting_power}'
traderswiftd query validatorset validators --node tcp://127.0.0.1:26657
```

The admin can also change your power (`set-validator-power`) or remove you (`remove-validator`). Removal takes effect two blocks later; your node keeps running as a full node with its copy of the chain.

## 8. Firewall

Open only p2p:

```
table inet filter {
  chain input {
    type filter hook input priority 0; policy drop;
    iif lo accept
    ct state established,related accept
    tcp dport 22 ip saddr <your admin ip> accept
    tcp dport 26656 accept
  }
}
```

RPC (26657), REST (1317), gRPC (9090) and pprof stay on loopback. Use an SSH tunnel to read them from elsewhere.

## 9. Monitoring

| Watch | How | Alert when |
|---|---|---|
| Height moving | `curl -s 127.0.0.1:26657/status \| jq -r .result.sync_info.latest_block_height` | Unchanged for 30 s |
| Catching up | `.result.sync_info.catching_up` | `true` for more than a few minutes |
| Peers | `curl -s 127.0.0.1:26657/net_info \| jq .result.n_peers` | Fewer than 2 |
| Signing | your `address` in `/commit?height=<h>` signatures, `block_id_flag` 2 | Missing for 10 blocks in a row |
| Disk, memory | the usual | Disk above 80% |
| Process | `systemctl is-active traderswift-chain` | Not active |

CometBFT also serves Prometheus metrics if you set `prometheus = true` under `[instrumentation]` in `config.toml`; keep that port on loopback too.

## 10. Upgrades and leaving

We announce a new release in this repository with its hash and the height to switch at. Watch the repository, and `networks/traderswift-1/README.md` for a fresh genesis. Stop, swap the binary, start. To leave, open an issue. We remove you, then you stop the node. Keep or delete your copy of the chain as you like; it holds no secret.

## 11. Joining a subnet as a provider: zero contact

Registration is open. Genesis seeds only the founding providers; no charter names a provider, and no one at TraderSwift approves a new one. The gates are a hub account with a claimed owner, and the owner's signature. You need a key of your own (its first transaction makes it a chain account; see "Your chain account" below), a hub account id, and any EVM wallet as the owner.

1. **Hub account.** Choose an `account_id` and an ed25519 registration key, and register it yourself or through any sponsor. `expiry` is a Unix time in the future; an unclaimed id past it can be taken again.
   ```
   traderswiftd tx network register-account --account-id <account_id> --registration-key <64 hex> --expiry <unix time> --from <your key> --chain-id traderswift-1
   ```
2. **Claim it** with your wallet: sign the claim text in the README (SIWE form) with the wallet (EIP-191) and with the registration key (ed25519), then send both signatures:
   ```
   traderswiftd tx network claim-owner --account-id <account_id> --owner-address <0x lowercase> --nonce <8-64 letters or digits> \
     --issued-at <unix> --expires-at <unix> --owner-signature <130 hex> --registration-signature <128 hex> --from <your key> --chain-id traderswift-1
   ```
3. **Register the provider**, signed by the provider address itself. The wallet signs the `register provider` text in the README over `subnet, provider_id, provider_kind, provider_address, endpoint`. Read the charter first (`/network/v1/charter/<subnet>`) for the allowed `provider_kinds` and the `join` rule.
   ```
   traderswiftd tx network register-provider --subnet timeseries --provider-id <your id> --provider-kind store \
     --provider-address <your chain address> --account-id <account_id> --endpoint "<public endpoint or empty>" \
     --owner-signature <130 hex> --from <your key> --chain-id traderswift-1
   ```
4. **Anchor.** From the provider address, anchor your segments as the subnet's charter says. They are accepted and stored from day one:
   ```
   traderswiftd tx network anchor --subnet timeseries --segment-kind candle_root --segment-id 2026-11-17 --segment-hash <sha256 hex> \
     --provider-id <your id> --from <your key> --chain-id traderswift-1
   ```

**Endorsement.** Under `join: open` you are endorsed at registration. Under `join: endorsed` (`timeseries` and `signer`) your anchors are on the record with `endorsed: false` and do not count toward the subnet's agreement rule. Once an endorsed provider of that subnet, or a charter operator, sends `endorse-provider --subnet <subnet> --provider-id <your id>`, new anchors carry `endorsed: true`. Each old anchor keeps the `endorsed` flag it had when sent. For `timeseries`, endorsing you once your `candle_root` anchors match the endorsed nodes' roots is operator policy; the code does not check roots. You do not message us. Watch it at `/network/v1/provider/<subnet>/<your id>` (`endorsed`, `endorsed_height`) and `/network/v1/endorsements/<subnet>/<your id>`.

**What can stop you.** `MsgVeto` from the network admin, on legal or regulatory grounds, with the reason public at `/network/v1/vetoes/<subnet>`. A vetoed provider's anchors are refused from then on; its past anchors stay. The admin's `SetCharter` can also drop a segment kind. Anchors whose `segment_kind` is not in the live charter are refused.

**Addresses.** Chain addresses use the bech32 prefix `traderswift`: accounts `traderswift1...`, validator operators `traderswiftvaloper1...`, consensus addresses `traderswiftvalcons1...`. A `cosmos1...` address, the default in most Cosmos wallets, is the same key under another prefix: the 20 bytes behind it are unchanged, so it re-encodes to one `traderswift1...` address with no new key and no new signature: `traderswiftd keys parse <cosmos1...>` prints the bytes, then `traderswiftd keys parse <those hex bytes>` prints the `traderswift1...` form. Use the `traderswift1...` form in every command and REST path here, for example `curl -s <rest>/network/v1/first_contact/traderswift1...` or `curl -s <rest>/cosmos/auth/v1beta1/accounts/traderswift1...`.

**Your chain account.** You need no sponsor and no token. A brand-new address becomes a chain account with its first transaction, if that transaction holds only `register-account`, `claim-owner` or `register-provider` and has sequence 0. Because the account does not exist yet, sign that first transaction offline with the account number the chain will give it, which is fixed by your address:
```
account_number=$(curl -s <rest>/network/v1/first_contact/<your chain address> | jq -r .account_number)
traderswiftd tx network register-account ... --from <your key> --generate-only --offline --account-number "$account_number" --sequence 0 --gas 400000 > unsigned.json
traderswiftd tx sign unsigned.json --from <your key> --chain-id traderswift-1 --offline --account-number "$account_number" --sequence 0 > signed.json
traderswiftd tx broadcast signed.json --node <rpc>
```
From then on send transactions normally. At most 20 first-contact signers get new chain accounts in one block; if yours is refused with `account_cap`, resend it in the next block. Provider accounts created through an operator's `RegisterProvider` do not count toward this limit. A first transaction with any other message (an `anchor`, say) is refused and creates nothing. An unclaimed hub account expires at its `expiry`; the chain account stays.

Being a provider gives you no vote in block production; that is section 7.
