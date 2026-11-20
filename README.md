# traderswift-chain

traderswift-chain is the shared record behind TraderSwift: who is who, which services exist, and the fingerprints of their data, kept by four machines that check each other's work. TraderSwift runs all four today, and outside validators can join. It holds no money and no token.

**Just want to see it? Start here:** [what the hub chain is](https://github.com/traderswift-io/traderswift-chain#traderswift-chain), then [the live network files](https://github.com/traderswift-io/traderswift-chain/tree/main/networks/traderswift-1): its chain ID, genesis and peers, readable in the browser. No install, no keys, no sign-up.

The `traderswift-1` hub chain: the common layer every subnet anchors onto. It holds accounts and identity, subnet charters, providers, vetoes, anchors and the validator set. It holds no balances, no token, and no subnet's data; a subnet publishes hashes here and keeps its content in its own store.

Join as a validator or a provider: `docs/VALIDATOR_JOIN.md`. Network files: `networks/traderswift-1/`.

Cosmos SDK v0.55.0, CometBFT v0.40.0. Modules: auth, consensus, genutil, validatorset, network. No bank, staking, mint, distribution or gov. Go 1.26.5, as in `go.mod`. Licence: Apache-2.0. `validatorset/` and the `RegisterAccount`/`ClaimOwner` handlers were copied from the Lockbox chain tree (same licence, same authors) and edited here; nothing is imported from it.

## Files

| Path | What |
|---|---|
| `network/` | The module: keeper, message servers, query server, REST routes, genesis, `make check` scenarios. `network.pb.go` is generated |
| `validatorset/` | The validator set: admin, validators, the one-third rule, updates to CometBFT. `validatorset.pb.go` is generated |
| `identitylogic/` | Field formats, owner message texts, EIP-191 and ed25519 checks. No SDK state |
| `proto/` | `network.proto`, `validatorset.proto`. Regenerate with `make proto` (needs `buf` and `protoc-gen-gocosmos` on `PATH`) |
| `main.go`, `app.go`, `export.go` | Node binary and wiring, from the SDK's Apache-2.0 `simapp`. The ante handler is the SDK's stock chain without the fee step |
| `chainprefix/` | The bech32 prefixes, `traderswift` for accounts, valoper and valcons, applied by the node and by key B before anything else |
| `cmd/hub_admin_key_b/` | Admin key B daemon, run by a founder: signs only the routine allowlist below. `hub-admin-key-b.service` is its unit (no `[Install]`) |
| `run_local_network.sh` | Local validators on `127.0.0.1`, ports `47000 + 10n` |

## Build and check

```
make build                 # build/traderswiftd
make check                 # gofmt, vet, scenarios, go.mod free of other chains
./run_local_network.sh [--directory path] up 4 | status | stop <n> | start <n> | down | wipe
```

Node `n` listens on `47000 + 10n`: `+0` p2p, `+1` CometBFT RPC, `+2` REST, `+3` gRPC, `+4` pprof. Keyring `<directory>/client`, backend `test`: `admin`, `sponsor`, `operator`, `outsider`. Keys are throwaway.

## Messages

| Message | Tx signer | Refused when |
|---|---|---|
| `RegisterAccount` (`account_id`, `registration_key`, `expiry`) | any chain account (the sponsor) | bad id or key, `expiry` not in the future, id taken (an unclaimed id past its expiry may be taken again) |
| `ClaimOwner` | any chain account | account unknown, already claimed, past `expiry`; owner EIP-191 or registration-key ed25519 signature does not verify over the claim text |
| `SetCharter` (`subnet`, `version`, `charter_hash`, `provider_kinds`, `segment_kinds`, `operators`, `join`) | network admin | not the admin, version not stored + 1, empty kinds, bad hash, `join` not `open`/`endorsed` |
| `RegisterProvider` (`subnet`, `provider_id`, `provider_kind`, `provider_address`, `account_id`, `endpoint`, `owner_signature`) | `provider_address` itself, or a charter operator | no live charter, kind not in it, account unknown or unclaimed, owner signature does not verify, id or address taken in the subnet |
| `EndorseProvider` (`subnet`, `provider_id`) | an endorsed, unvetoed provider of the same subnet, or a charter operator | no live charter, submitter neither, target unknown or vetoed, same endorser twice |
| `Veto` (`subnet`, `target_kind`, `target_id`, `reason_code`, `reason`) | network admin | not the admin, kind not `charter`/`provider`/`anchor`, reason code not `legal`/`regulatory`, empty reason, target missing, already vetoed |
| `Anchor` (`subnet`, `segment_kind`, `segment_id`, `segment_hash`, `provider_id`) | the provider's `provider_address` | no live charter, kind not in it, provider unknown or vetoed, wrong signer, key used |

Registration is open: genesis seeds the founding providers (six `timeseries` providers: three nodes, each a `store` and a `serve` provider); every later provider registers itself, and no charter names one. The gates are the hub account (it exists, its `owner_address` is claimed) and the owner's signature. Operators are extra registrars, never the only ones.

Registration records; endorsement counts. `Provider.endorsed_height` is 0 until endorsed. Under `join: open` a provider is endorsed at registration. Under `join: endorsed` the first `EndorseProvider` sets it; every endorsement is stored as `Endorsement{subnet, provider_id, endorser_id, height}`, where `endorser_id` is the endorsing provider's id or the operator's address. Anchors are accepted from every registered, unvetoed provider either way, and each `Anchor` carries `endorsed`, the provider's state when it anchored. A subnet's agreement rule counts only `endorsed` anchors; the chain stores, it does not count.

The network admin is the validator-set admin, fixed in genesis; a multisig works. Accounts come from genesis, from `RegisterProvider` (which creates the provider address's account), or from first contact.

First contact (`network/ante_first_contact.go`, ante decorator before the public-key step): a signer with no chain account gets one only when every message in the tx is `RegisterAccount`, `ClaimOwner` or `RegisterProvider`, its sequence is 0, and fewer than 20 accounts have been created in this block; over the cap the tx is refused with `account_cap` and is resent next block. Any other first tx from an unknown address is refused. The new account's number is fixed by its address, the first 8 bytes of sha256(`traderswift first contact account number ` + address bytes) with the top bit set, so the newcomer can sign offline before the account exists; `/network/v1/first_contact/{address}` returns it. If any account already holds that number, the tx is refused with `account_number_taken` and nothing is created. Hub `Account` rows keep their starter expiry, so unclaimed ones age out; the auth account stays. No sponsor is needed.

`subnet`, `provider_id`, `provider_kind` and the kinds are 1 to 64 of letters, digits, dot, underscore, dash: no colon, so an anchor veto's `target_id`, `segment_kind:segment_id:provider_id`, splits on the first and last colon. `segment_id` and `account_id` may hold colons. A charter veto's `target_id` is the version number.

What a veto does: a vetoed charter version refuses new providers and anchors until the next version is set; a vetoed provider's anchors are refused from then on; a vetoed anchor stays stored with `vetoed: true`. Nothing is deleted.

Events: `network.account_registered`, `network.claimed`, `network.charter_set`, `network.provider_registered` (with `endorsed`), `network.provider_endorsed`, `network.vetoed`, `network.anchored` (with `endorsed`).

## Texts the owner wallet signs

EIP-191 `personal_sign`, 65-byte signature as 130 lowercase hex. Line breaks `\n`, no trailing newline.

Claim, SIWE form. The address is EIP-55 here, lowercase in the message field; times are UTC RFC 3339. The registration key signs the same text with ed25519.

```
traderswift wants you to sign in with your Ethereum account:
<EIP-55 owner address>

Claim traderswift account <account_id> with registration key <registration_key>

URI: traderswift://<chain id>/claim
Version: 1
Chain ID: 1
Nonce: <8 to 64 letters or digits>
Issued At: <issued_at>
Expiration Time: <expires_at>
```

```
traderswift register provider
chain id: <chain id>
subnet: <subnet>
provider id: <provider_id>
provider kind: <provider_kind>
provider address: <provider_address>
endpoint: <endpoint, may be empty>
```

## REST

All `GET`, flat JSON, numbers as numbers, empty lists `[]`, missing is 404 with `{"error": ...}`.

| Path | Returns |
|---|---|
| `/network/v1/account/{account_id}` | `account_id`, `registration_key`, `owner_address`, `expiry` |
| `/network/v1/charter/{subnet}[/{version}]` | the current or a given charter, with `join` and `vetoed` |
| `/network/v1/provider/{subnet}/{provider_id}` | one provider, with `endorsed_height`, `endorsed` and `vetoed` |
| `/network/v1/providers/{subnet}[?provider_kind=]` | list, each with `endorsed` |
| `/network/v1/endorsements/{subnet}/{provider_id}` | who endorsed the provider, and at what height |
| `/network/v1/anchors/{subnet}/{segment_kind}/{segment_id}` | every provider's anchor for the segment, each with `endorsed`; agreement is read, not computed |
| `/network/v1/anchor/{subnet}/{segment_kind}/{segment_id}/{provider_id}` | one anchor |
| `/network/v1/first_contact/{address}` | what to sign with: for an existing account its real `account_number` and `sequence` (`exists: true`); otherwise the derived number and `sequence` 0 (`exists: false`). `account_number` is a string: it can exceed 2^53 |
| `/network/v1/veto/{subnet}/{target_kind}/{target_id}`, `/network/v1/vetoes/{subnet}` | veto rows with reason |

## Genesis

`app_state.validatorset = {admin, validators: [{consensus_public_key, power, moniker, founding}]}`, as in the validator set's rules (one non-founding validator under a third of the power). `app_state.network = {accounts, charters, providers, vetoes, anchors, endorsements}`; charters list every version in order, each with `join`. A genesis provider is endorsed at genesis by giving it `endorsed_height` 1 (and, if wanted, an `endorsements` row naming the endorser). `genesis validate` checks formats, version order, `join`, that providers point at a charter kind and a claimed account, and that endorsements point at an endorsed provider. Chain accounts go in `app_state.auth.accounts` with no balance.

## Admin keys: 2-of-3, and key B

The admin in genesis is a 2-of-3 `LegacyAminoPubKey` multisig of three keys held by the founders on separate machines: A, B (the daemon below) and C. Every admin tx is signed `SIGN_MODE_LEGACY_AMINO_JSON` and combined with `traderswiftd tx multi-sign`. Addresses use the chain's bech32 prefix `traderswift` (`chainprefix/`: `traderswift1`, `traderswiftvaloper1`, `traderswiftvalcons1`); a `cosmos1...` address is the same key re-encoded.

Key B allowlist, fixed in `cmd/hub_admin_key_b/allowlist.go` (a change is a new binary):

| Message | Condition |
|---|---|
| `/traderswift.network.v1.MsgEndorseProvider` | `submitter` is the admin |
| `/traderswift.network.v1.MsgRegisterProvider` | `submitter` is the admin |
| `/traderswift.network.v1.MsgSetCharter` | `submitter` is the admin |
| `/traderswift.validatorset.v1.MsgAddValidator` | `admin` is the admin, `founding` false |

Everything else is refused, always `MsgVeto`, `MsgRemoveValidator` and `MsgSetValidatorPower` (and a founding `MsgAddValidator`). Also refused: a tx with a nonzero fee, a fee payer other than `admin_address` or a fee granter, any message whose signer is not `admin_address`, an empty tx. The admin may be the fee payer when the fee is zero and there is no fee granter. Those three need key C first, then key A.

```
hub_admin_key_b keygen --key-file /var/lib/traderswift/hub_admin_key_b/key_b.hex   # 0600, O_EXCL: never overwrites; prints address and pubkey only
hub_admin_key_b public --key-file <file>                                         # address and pubkey again
hub_admin_key_b sign-doc --unsigned-tx unsigned.json --chain-id traderswift-1 --admin <multisig> --account-number n --sequence s
                                                                                   # keyless: the amino-JSON sign bytes, base64, for key A's request
hub_admin_key_b serve --config /etc/traderswift/hub_admin_key_b.json               # {"listen": "127.0.0.1:47900", "chain_id", "admin_address", "key_file"}
```

`serve` listens on loopback only (anything else is refused at start; reach it with `ssh -L`). `POST /sign` takes `{request_id, unsigned_tx (the --generate-only JSON), account_number, sequence}` as strings for the numbers, decodes the tx with the chain's own types, checks the allowlist, and answers 200 with a `tx multi-sign` signature file, or 403 `{request_id, refused}`. One log line per request with the message types; never the key. The key file must be 0600.

## Not built

`RevokeEndorsement` and `RetireProvider` (planned), owner rotation, `owner_identity`, changing the admin after genesis, `x/upgrade`, state sync snapshots.
