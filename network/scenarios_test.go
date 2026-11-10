package network

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"

	errorsmod "cosmossdk.io/errors"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"traderswift-chain/identitylogic"
)

type fixedAdminSource struct{ admin string }

func (source fixedAdminSource) Admin(sdk.Context) (string, error) { return source.admin, nil }

type recordingAccountCreator struct {
	created        map[string]bool
	accountNumbers map[string]uint64
}

func (creator *recordingAccountCreator) HasAccount(_ context.Context, address sdk.AccAddress) bool {
	return creator.created[address.String()]
}

func (creator *recordingAccountCreator) HasAccountNumber(_ context.Context, accountNumber uint64) (bool, error) {
	for _, heldNumber := range creator.accountNumbers {
		if heldNumber == accountNumber {
			return true, nil
		}
	}
	return false, nil
}

func (creator *recordingAccountCreator) NewAccountWithAddress(_ context.Context, address sdk.AccAddress) sdk.AccountI {
	return authtypes.NewBaseAccountWithAddress(address)
}

func (creator *recordingAccountCreator) SetAccount(_ context.Context, account sdk.AccountI) {
	creator.created[account.GetAddress().String()] = true
	if creator.accountNumbers == nil {
		creator.accountNumbers = map[string]uint64{}
	}
	creator.accountNumbers[account.GetAddress().String()] = account.GetAccountNumber()
}

type ownerWallet struct {
	privateKey *secp256k1.PrivateKey
	address    string
}

func newOwnerWallet(seed byte) ownerWallet {
	privateKey := secp256k1.PrivKeyFromBytes(sha256Bytes([]byte{seed}))
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(privateKey.PubKey().SerializeUncompressed()[1:])
	return ownerWallet{privateKey: privateKey, address: "0x" + hex.EncodeToString(hasher.Sum(nil)[12:])}
}

func (wallet ownerWallet) signEip191(text string) string {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(text), text)))
	compact := secp256k1ecdsa.SignCompact(wallet.privateKey, hasher.Sum(nil), false)
	return hex.EncodeToString(append(compact[1:], compact[0]))
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func sha256Hex(text string) string {
	return hex.EncodeToString(sha256Bytes([]byte(text)))
}

func chainAddress(seed string) string {
	return sdk.AccAddress(sha256Bytes([]byte(seed))[:20]).String()
}

type scenarioRun struct {
	t      *testing.T
	ctx    sdk.Context
	server MsgServer
	query  QueryServer
	keeper *Keeper
	passed int
	failed int
}

func (run *scenarioRun) accepted(name string, err error) {
	if err != nil {
		run.failed++
		run.t.Errorf("FAIL %s: refused: %v", name, err)
		return
	}
	run.passed++
}

func (run *scenarioRun) refused(name string, err error, wanted error) {
	if err == nil || (wanted != nil && !errorsmod.IsOf(err, wanted)) {
		run.failed++
		run.t.Errorf("FAIL %s: wanted refusal %v, got %v", name, wanted, err)
		return
	}
	run.passed++
}

func (run *scenarioRun) check(name string, condition bool) {
	if !condition {
		run.failed++
		run.t.Errorf("FAIL %s", name)
		return
	}
	run.passed++
}

func newScenarioRun(t *testing.T, admin string, creator *recordingAccountCreator) *scenarioRun {
	storeKey := storetypes.NewKVStoreKey(StoreKey)
	ctx := testutil.DefaultContext(storeKey, storetypes.NewTransientStoreKey("transient_network"))
	ctx = ctx.WithChainID("traderswift-1").WithBlockHeight(10).WithBlockTime(time.Unix(1_800_000_000, 0))
	encoding := moduletestutil.MakeTestEncodingConfig()
	keeper := NewKeeper(encoding.Codec, runtime.NewKVStoreService(storeKey), fixedAdminSource{admin: admin}, creator)
	return &scenarioRun{t: t, ctx: ctx, server: NewMsgServer(keeper), query: NewQueryServer(keeper), keeper: keeper}
}

func claimMessage(run *scenarioRun, accountId string, registrationKey ed25519.PrivateKey, owner ownerWallet, signer ownerWallet) *MsgClaimOwner {
	issuedAt := uint64(run.ctx.BlockTime().Unix()) - 60
	claim := identitylogic.OwnerClaim{ChainId: "traderswift-1", AccountId: accountId, RegistrationKey: hex.EncodeToString(registrationKey.Public().(ed25519.PublicKey)),
		OwnerAddress: owner.address, Nonce: "nonce12345", IssuedAt: issuedAt, ExpiresAt: issuedAt + 600}
	text, _ := identitylogic.OwnerClaimText(claim)
	return &MsgClaimOwner{Submitter: chainAddress("sponsor"), AccountId: accountId, OwnerAddress: owner.address, Nonce: claim.Nonce,
		IssuedAt: claim.IssuedAt, ExpiresAt: claim.ExpiresAt, OwnerSignature: signer.signEip191(text),
		RegistrationSignature: hex.EncodeToString(ed25519.Sign(registrationKey, []byte(text)))}
}

func providerMessage(submitter string, subnet string, providerId string, kind string, providerAddress string, accountId string, owner ownerWallet) *MsgRegisterProvider {
	text := identitylogic.ProviderRegistrationText(identitylogic.ProviderRegistration{ChainId: "traderswift-1", Subnet: subnet, ProviderId: providerId,
		ProviderKind: kind, ProviderAddress: providerAddress})
	return &MsgRegisterProvider{Submitter: submitter, Subnet: subnet, ProviderId: providerId, ProviderKind: kind, ProviderAddress: providerAddress,
		AccountId: accountId, OwnerSignature: owner.signEip191(text)}
}

func TestNetworkScenarios(t *testing.T) {
	admin := chainAddress("admin")
	stranger := chainAddress("stranger")
	creator := &recordingAccountCreator{created: map[string]bool{}}
	run := newScenarioRun(t, admin, creator)
	ctx := run.ctx
	owner := newOwnerWallet(1)
	otherWallet := newOwnerWallet(2)
	_, registrationKey, _ := ed25519.GenerateKey(nil)
	_, otherRegistrationKey, _ := ed25519.GenerateKey(nil)
	registrationKeyHex := hex.EncodeToString(registrationKey.Public().(ed25519.PublicKey))
	now := uint64(ctx.BlockTime().Unix())

	_, err := run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "example-account", RegistrationKey: registrationKeyHex, Expiry: now + 86400})
	run.accepted("RegisterAccount by a sponsor", err)
	_, err = run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "example-account", RegistrationKey: registrationKeyHex, Expiry: now + 86400})
	run.refused("RegisterAccount duplicate", err, ErrAlreadyExists)
	_, err = run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "late", RegistrationKey: registrationKeyHex, Expiry: now})
	run.refused("RegisterAccount expiry not in the future", err, ErrInvalidField)
	_, err = run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "badkey", RegistrationKey: "00", Expiry: now + 60})
	run.refused("RegisterAccount bad registration key", err, ErrInvalidField)
	_, err = run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "bad id!", RegistrationKey: registrationKeyHex, Expiry: now + 60})
	run.refused("RegisterAccount bad account id", err, ErrInvalidField)

	_, err = run.server.ClaimOwner(ctx, claimMessage(run, "example-account", registrationKey, owner, otherWallet))
	run.refused("ClaimOwner signed by another wallet", err, ErrUnauthorized)
	_, err = run.server.ClaimOwner(ctx, claimMessage(run, "example-account", otherRegistrationKey, owner, owner))
	run.refused("ClaimOwner signed by another registration key", err, ErrUnauthorized)
	_, err = run.server.ClaimOwner(ctx, claimMessage(run, "nobody", registrationKey, owner, owner))
	run.refused("ClaimOwner unknown account", err, ErrNotFound)
	_, err = run.server.ClaimOwner(ctx, claimMessage(run, "example-account", registrationKey, owner, owner))
	run.accepted("ClaimOwner SIWE + registration key", err)
	_, err = run.server.ClaimOwner(ctx, claimMessage(run, "example-account", registrationKey, otherWallet, otherWallet))
	run.refused("ClaimOwner second claim", err, ErrAlreadyExists)
	accountResponse, _ := run.query.Account(ctx, &QueryAccountRequest{AccountId: "example-account"})
	run.check("Account shows owner", accountResponse != nil && accountResponse.Account.OwnerAddress == owner.address)

	_, err = run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "starter", RegistrationKey: registrationKeyHex, Expiry: now + 100})
	run.accepted("RegisterAccount short starter", err)
	laterCtx := ctx.WithBlockTime(time.Unix(int64(now+200), 0))
	laterRun := *run
	laterRun.ctx = laterCtx
	_, err = run.server.ClaimOwner(laterCtx, claimMessage(&laterRun, "starter", registrationKey, owner, owner))
	run.refused("ClaimOwner after expiry", err, ErrExpired)
	_, err = run.server.RegisterAccount(laterCtx, &MsgRegisterAccount{Submitter: stranger, AccountId: "starter", RegistrationKey: registrationKeyHex, Expiry: now + 1000})
	run.accepted("RegisterAccount reuses an expired unclaimed id", err)
	_, err = run.server.RegisterAccount(laterCtx, &MsgRegisterAccount{Submitter: stranger, AccountId: "example-account", RegistrationKey: registrationKeyHex, Expiry: now + 1000})
	run.refused("RegisterAccount never reuses a claimed id", err, ErrAlreadyExists)

	_, err = run.server.RegisterAccount(ctx, &MsgRegisterAccount{Submitter: stranger, AccountId: "unclaimed", RegistrationKey: registrationKeyHex, Expiry: now + 86400})
	run.accepted("RegisterAccount unclaimed", err)

	charterHash := sha256Hex("charts charter v1")
	operator := chainAddress("operator")
	charter := &MsgSetCharter{Submitter: admin, Subnet: "charts", Version: 1, CharterHash: charterHash, ProviderKinds: []string{"data_node"}, SegmentKinds: []string{"meter", "candle_root"}, Operators: []string{operator}, Join: "endorsed"}
	nonAdminCharter := *charter
	nonAdminCharter.Submitter = stranger
	_, err = run.server.SetCharter(ctx, &nonAdminCharter)
	run.refused("SetCharter by non-admin", err, ErrUnauthorized)
	skipped := *charter
	skipped.Version = 2
	_, err = run.server.SetCharter(ctx, &skipped)
	run.refused("SetCharter version skip", err, ErrInvalidField)
	noKinds := *charter
	noKinds.SegmentKinds = nil
	_, err = run.server.SetCharter(ctx, &noKinds)
	run.refused("SetCharter empty segment kinds", err, ErrInvalidField)
	badHash := *charter
	badHash.CharterHash = "ABC"
	_, err = run.server.SetCharter(ctx, &badHash)
	run.refused("SetCharter bad hash", err, ErrInvalidField)
	noJoin := *charter
	noJoin.Join = ""
	_, err = run.server.SetCharter(ctx, &noJoin)
	run.refused("SetCharter join neither open nor endorsed", err, ErrInvalidField)
	_, err = run.server.SetCharter(ctx, charter)
	run.accepted("SetCharter charts v1", err)
	_, err = run.server.SetCharter(ctx, charter)
	run.refused("SetCharter same version again", err, ErrInvalidField)
	_, err = run.server.SetCharter(ctx, &MsgSetCharter{Submitter: admin, Subnet: "lockbox", Version: 1, CharterHash: sha256Hex("lockbox v1"), ProviderKinds: []string{"validator", "coordinator"}, SegmentKinds: []string{"state", "meter"}, Join: "endorsed"})
	run.accepted("SetCharter lockbox v1, no operators", err)
	_, err = run.server.SetCharter(ctx, &MsgSetCharter{Submitter: admin, Subnet: "relay", Version: 1, CharterHash: sha256Hex("relay v1"), ProviderKinds: []string{"relay_node"}, SegmentKinds: []string{"names"}, Join: "open"})
	run.accepted("SetCharter relay v1, join open", err)

	nodeOne, nodeTwo, nodeThree := chainAddress("node-1"), chainAddress("node-2"), chainAddress("node-3")
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeOne, "charts", "charts-node-1", "data_node", nodeOne, "example-account", owner))
	run.accepted("RegisterProvider self with a claimed account", err)
	run.check("RegisterProvider creates the provider's chain account", creator.created[nodeOne])
	providerResponse, _ := run.query.Provider(ctx, &QueryProviderRequest{Subnet: "charts", ProviderId: "charts-node-1"})
	run.check("Provider under join endorsed starts unendorsed", providerResponse != nil && providerResponse.Provider.EndorsedHeight == 0)
	_, err = run.server.RegisterProvider(ctx, providerMessage(operator, "charts", "charts-node-2", "data_node", nodeTwo, "example-account", owner))
	run.accepted("RegisterProvider by charter operator", err)
	_, err = run.server.RegisterProvider(ctx, providerMessage(stranger, "charts", "charts-node-3", "data_node", nodeThree, "example-account", owner))
	run.refused("RegisterProvider by stranger", err, ErrUnauthorized)
	_, err = run.server.RegisterProvider(ctx, providerMessage(stranger, "lockbox", "lockbox-1", "validator", nodeThree, "example-account", owner))
	run.refused("RegisterProvider by stranger when charter has no operators", err, ErrUnauthorized)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "charts", "charts-node-3", "signer", nodeThree, "example-account", owner))
	run.refused("RegisterProvider kind not in charter", err, ErrInvalidField)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "charts", "charts-node-3", "data_node", nodeThree, "example-account", otherWallet))
	run.refused("RegisterProvider owner signature from another wallet", err, ErrUnauthorized)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "charts", "charts-node-3", "data_node", nodeThree, "unclaimed", owner))
	run.refused("RegisterProvider self with an unclaimed account", err, ErrNotClaimed)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "charts", "charts-node-3", "data_node", nodeThree, "nobody", owner))
	run.refused("RegisterProvider self with an unknown account", err, ErrNotFound)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "context", "context-1", "data_node", nodeThree, "example-account", owner))
	run.refused("RegisterProvider subnet without charter", err, ErrNotFound)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "charts", "charts-node-1", "data_node", nodeThree, "example-account", owner))
	run.refused("RegisterProvider duplicate provider id", err, ErrAlreadyExists)
	_, err = run.server.RegisterProvider(ctx, providerMessage(operator, "charts", "charts-node-9", "data_node", nodeOne, "example-account", owner))
	run.refused("RegisterProvider duplicate provider address", err, ErrAlreadyExists)
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeThree, "charts", "charts-node-3", "data_node", nodeThree, "example-account", owner))
	run.accepted("RegisterProvider third node", err)
	nodeFour := chainAddress("node-4")
	_, err = run.server.RegisterProvider(ctx, providerMessage(nodeFour, "relay", "relay-1", "relay_node", nodeFour, "example-account", owner))
	run.accepted("RegisterProvider self under join open", err)
	providerResponse, _ = run.query.Provider(ctx, &QueryProviderRequest{Subnet: "relay", ProviderId: "relay-1"})
	run.check("Provider under join open is endorsed at registration", providerResponse != nil && providerResponse.Provider.EndorsedHeight == 10)

	day := "2026-11-17"
	rootOne := sha256Hex("root")
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "candle_root", SegmentId: day, SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.accepted("Anchor node 1", err)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeTwo, Subnet: "charts", SegmentKind: "candle_root", SegmentId: day, SegmentHash: rootOne, ProviderId: "charts-node-2"})
	run.accepted("Anchor node 2, same segment", err)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "candle_root", SegmentId: day, SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.refused("Anchor duplicate key", err, ErrAlreadyExists)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "tape", SegmentId: day, SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.refused("Anchor kind not in charter", err, ErrInvalidField)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: stranger, Subnet: "charts", SegmentKind: "meter", SegmentId: "cht-2026-11-17-1", SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.refused("Anchor from unregistered address", err, ErrUnauthorized)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "meter", SegmentId: "cht-2026-11-17-1", SegmentHash: "xyz", ProviderId: "charts-node-1"})
	run.refused("Anchor bad hash", err, ErrInvalidField)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "meter", SegmentId: "2026-11-17:12345", SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.accepted("Anchor segment id with colon", err)
	anchorsResponse, _ := run.query.Anchors(ctx, &QueryAnchorsRequest{Subnet: "charts", SegmentKind: "candle_root", SegmentId: day})
	run.check("Anchors returns both providers' hashes", anchorsResponse != nil && len(anchorsResponse.Anchors) == 2 && anchorsResponse.Anchors[0].SegmentHash == anchorsResponse.Anchors[1].SegmentHash)
	run.check("Anchor from a registered, unendorsed provider is stored with endorsed false", anchorsResponse != nil && len(anchorsResponse.Anchors) == 2 && !anchorsResponse.Anchors[0].Endorsed && !anchorsResponse.Anchors[1].Endorsed)

	endorseCtx := ctx.WithBlockHeight(12)
	_, err = run.server.EndorseProvider(endorseCtx, &MsgEndorseProvider{Submitter: nodeOne, Subnet: "charts", ProviderId: "charts-node-2"})
	run.refused("EndorseProvider by an unendorsed provider", err, ErrUnauthorized)
	_, err = run.server.EndorseProvider(endorseCtx, &MsgEndorseProvider{Submitter: nodeFour, Subnet: "charts", ProviderId: "charts-node-2"})
	run.refused("EndorseProvider by an endorsed provider of another subnet", err, ErrUnauthorized)
	_, err = run.server.EndorseProvider(endorseCtx, &MsgEndorseProvider{Submitter: stranger, Subnet: "charts", ProviderId: "charts-node-2"})
	run.refused("EndorseProvider by a stranger", err, ErrUnauthorized)
	_, err = run.server.EndorseProvider(endorseCtx, &MsgEndorseProvider{Submitter: operator, Subnet: "charts", ProviderId: "charts-node-7"})
	run.refused("EndorseProvider of an unregistered provider", err, ErrNotFound)
	_, err = run.server.EndorseProvider(endorseCtx, &MsgEndorseProvider{Submitter: operator, Subnet: "charts", ProviderId: "charts-node-1"})
	run.accepted("EndorseProvider by a charter operator", err)
	_, err = run.server.EndorseProvider(endorseCtx, &MsgEndorseProvider{Submitter: operator, Subnet: "charts", ProviderId: "charts-node-1"})
	run.refused("EndorseProvider same endorser twice", err, ErrAlreadyExists)
	_, err = run.server.EndorseProvider(ctx.WithBlockHeight(13), &MsgEndorseProvider{Submitter: nodeOne, Subnet: "charts", ProviderId: "charts-node-2"})
	run.accepted("EndorseProvider by an endorsed provider of the same subnet", err)
	_, err = run.server.EndorseProvider(ctx.WithBlockHeight(14), &MsgEndorseProvider{Submitter: operator, Subnet: "charts", ProviderId: "charts-node-2"})
	run.accepted("EndorseProvider second endorser", err)
	providerResponse, _ = run.query.Provider(ctx, &QueryProviderRequest{Subnet: "charts", ProviderId: "charts-node-2"})
	run.check("First endorsement sets endorsed_height, later ones keep it", providerResponse != nil && providerResponse.Provider.EndorsedHeight == 13)
	endorsementsResponse, _ := run.query.Endorsements(ctx, &QueryEndorsementsRequest{Subnet: "charts", ProviderId: "charts-node-2"})
	run.check("Endorsements lists both endorsers", endorsementsResponse != nil && len(endorsementsResponse.Endorsements) == 2)
	providersResponse, _ := run.query.Providers(ctx, &QueryProvidersRequest{Subnet: "charts", ProviderKind: "data_node"})
	run.check("Providers(subnet, kind) shows who is endorsed", providersResponse != nil && len(providersResponse.Providers) == 3 &&
		providersResponse.Providers[0].EndorsedHeight == 12 && providersResponse.Providers[1].EndorsedHeight == 13 && providersResponse.Providers[2].EndorsedHeight == 0)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "candle_root", SegmentId: "2026-11-16", SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.accepted("Anchor after endorsement", err)
	anchorResponse, _ := run.query.Anchor(ctx, &QueryAnchorRequest{Subnet: "charts", SegmentKind: "candle_root", SegmentId: "2026-11-16", ProviderId: "charts-node-1"})
	run.check("Anchor after endorsement is stored with endorsed true", anchorResponse != nil && anchorResponse.Anchor.Endorsed)

	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "provider", TargetId: "charts-node-3", ReasonCode: "commercial", Reason: "no"})
	run.refused("Veto reason code off the list", err, ErrInvalidField)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "provider", TargetId: "charts-node-3", ReasonCode: "legal", Reason: ""})
	run.refused("Veto empty reason", err, ErrInvalidField)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: stranger, Subnet: "charts", TargetKind: "provider", TargetId: "charts-node-3", ReasonCode: "legal", Reason: "court order"})
	run.refused("Veto by non-admin", err, ErrUnauthorized)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "provider", TargetId: "charts-node-7", ReasonCode: "legal", Reason: "court order"})
	run.refused("Veto unknown provider", err, ErrNotFound)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "subnet", TargetId: "charts", ReasonCode: "legal", Reason: "court order"})
	run.refused("Veto unknown target kind", err, ErrInvalidField)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "provider", TargetId: "charts-node-3", ReasonCode: "regulatory", Reason: "licence withdrawn"})
	run.accepted("Veto provider", err)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "provider", TargetId: "charts-node-3", ReasonCode: "legal", Reason: "again"})
	run.refused("Veto twice", err, ErrAlreadyExists)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeThree, Subnet: "charts", SegmentKind: "candle_root", SegmentId: day, SegmentHash: rootOne, ProviderId: "charts-node-3"})
	run.refused("Anchor by vetoed provider", err, ErrVetoed)
	_, err = run.server.EndorseProvider(ctx, &MsgEndorseProvider{Submitter: operator, Subnet: "charts", ProviderId: "charts-node-3"})
	run.refused("EndorseProvider of a vetoed provider", err, ErrVetoed)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "anchor", TargetId: "meter:2026-11-17:12345:charts-node-1", ReasonCode: "legal", Reason: "court order"})
	run.accepted("Veto anchor, segment id with colon", err)
	anchorResponse, _ = run.query.Anchor(ctx, &QueryAnchorRequest{Subnet: "charts", SegmentKind: "meter", SegmentId: "2026-11-17:12345", ProviderId: "charts-node-1"})
	run.check("Vetoed anchor stays stored, marked vetoed", anchorResponse != nil && anchorResponse.Anchor.Vetoed)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "charter", TargetId: "2", ReasonCode: "legal", Reason: "court order"})
	run.refused("Veto charter version that does not exist", err, ErrNotFound)
	_, err = run.server.Veto(ctx, &MsgVeto{Submitter: admin, Subnet: "charts", TargetKind: "charter", TargetId: "1", ReasonCode: "legal", Reason: "court order"})
	run.accepted("Veto charter v1", err)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "candle_root", SegmentId: "2026-11-18", SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.refused("Anchor under vetoed charter", err, ErrVetoed)
	_, err = run.server.RegisterProvider(ctx, providerMessage(operator, "charts", "charts-node-4", "data_node", chainAddress("node-4"), "example-account", owner))
	run.refused("RegisterProvider under vetoed charter", err, ErrVetoed)
	charterTwo := *charter
	charterTwo.Version = 2
	charterTwo.CharterHash = sha256Hex("charts charter v2")
	_, err = run.server.SetCharter(ctx, &charterTwo)
	run.accepted("SetCharter charts v2 after veto", err)
	_, err = run.server.Anchor(ctx, &MsgAnchor{Submitter: nodeOne, Subnet: "charts", SegmentKind: "candle_root", SegmentId: "2026-11-18", SegmentHash: rootOne, ProviderId: "charts-node-1"})
	run.accepted("Anchor under charter v2", err)
	charterResponse, _ := run.query.Charter(ctx, &QueryCharterRequest{Subnet: "charts", Version: 1})
	run.check("Old charter version kept and marked vetoed", charterResponse != nil && charterResponse.Charter.CharterHash == charterHash && charterResponse.Vetoed)

	exported, err := run.keeper.ExportToGenesis(ctx)
	run.accepted("Export genesis", err)
	run.accepted("Exported genesis validates", ValidateGenesisState(exported))
	reimported := newScenarioRun(t, admin, &recordingAccountCreator{created: map[string]bool{}})
	run.accepted("Exported genesis imports", reimported.keeper.InitializeFromGenesis(reimported.ctx, exported))
	reexported, _ := reimported.keeper.ExportToGenesis(reimported.ctx)
	run.check("Re-export matches export", reexported != nil && reexported.String() == exported.String())
	brokenGenesis := &GenesisState{Charters: []*Charter{{Subnet: "charts", Version: 2, CharterHash: charterHash, ProviderKinds: []string{"a"}, SegmentKinds: []string{"b"}, Join: "endorsed"}}}
	run.refused("Genesis with a charter version gap", ValidateGenesisState(brokenGenesis), nil)
	endorsementOfUnknown := &GenesisState{Endorsements: []*Endorsement{{Subnet: "charts", ProviderId: "charts-node-1", EndorserId: operator, Height: 1}}}
	run.refused("Genesis endorsement of an unknown provider", ValidateGenesisState(endorsementOfUnknown), nil)
	genesisEndorsed := &GenesisState{
		Accounts:     []*Account{{AccountId: "example-account", RegistrationKey: registrationKeyHex, OwnerAddress: owner.address}},
		Charters:     []*Charter{{Subnet: "charts", Version: 1, CharterHash: charterHash, ProviderKinds: []string{"data_node"}, SegmentKinds: []string{"candle_root"}, Join: "endorsed"}},
		Providers:    []*Provider{{Subnet: "charts", ProviderId: "charts-node-1", ProviderKind: "data_node", ProviderAddress: nodeOne, AccountId: "example-account", Height: 1, EndorsedHeight: 1}},
		Endorsements: []*Endorsement{{Subnet: "charts", ProviderId: "charts-node-1", EndorserId: "genesis", Height: 1}},
	}
	run.accepted("Genesis provider endorsed at genesis", ValidateGenesisState(genesisEndorsed))

	t.Logf("network scenarios: %d of %d passed", run.passed, run.passed+run.failed)
	fmt.Printf("network scenarios: %d of %d passed\n", run.passed, run.passed+run.failed)
}
