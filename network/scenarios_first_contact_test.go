package network

import (
	"fmt"
	"testing"
	"time"

	protov2 "google.golang.org/protobuf/proto"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	txsigning "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

type syntheticSignedTx struct {
	messages  []sdk.Msg
	signers   [][]byte
	sequences []uint64
}

func (tx syntheticSignedTx) GetMsgs() []sdk.Msg                    { return tx.messages }
func (tx syntheticSignedTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }
func (tx syntheticSignedTx) GetSigners() ([][]byte, error)         { return tx.signers, nil }

func (tx syntheticSignedTx) GetSignaturesV2() ([]txsigning.SignatureV2, error) {
	signatures := []txsigning.SignatureV2{}
	for _, sequence := range tx.sequences {
		signatures = append(signatures, txsigning.SignatureV2{Sequence: sequence})
	}
	return signatures, nil
}

func signedBy(seed string, sequence uint64, messages ...sdk.Msg) syntheticSignedTx {
	return syntheticSignedTx{messages: messages, signers: [][]byte{sdk.MustAccAddressFromBech32(chainAddress(seed))}, sequences: []uint64{sequence}}
}

func TestFirstContactScenarios(t *testing.T) {
	admin := chainAddress("admin")
	creator := &recordingAccountCreator{created: map[string]bool{chainAddress("known"): true}}
	run := newScenarioRun(t, admin, creator)
	decorator := NewFirstContactDecorator(run.keeper)
	reachedNext := false
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		reachedNext = true
		return ctx, nil
	}
	ante := func(ctx sdk.Context, tx sdk.Tx) error {
		reachedNext = false
		_, err := decorator.AnteHandle(ctx, tx, false, next)
		return err
	}
	registrationKeyHex := fmt.Sprintf("%064x", 7)
	expiry := uint64(run.ctx.BlockTime().Unix()) + 86400
	newcomer := chainAddress("newcomer")
	registerAccount := &MsgRegisterAccount{Submitter: newcomer, AccountId: "newcomer-node", RegistrationKey: registrationKeyHex, Expiry: expiry}
	anchor := &MsgAnchor{Submitter: chainAddress("stranger-anchor"), Subnet: "charts", SegmentKind: "candle_root", SegmentId: "2026-11-17", SegmentHash: sha256Hex("x"), ProviderId: "charts-node-1"}

	err := ante(run.ctx, signedBy("newcomer", 0, registerAccount))
	run.accepted("Unknown address, first tx RegisterAccount, passes the ante", err)
	run.check("Unknown address now exists as a chain account", creator.created[newcomer] && reachedNext)
	run.check("New account number is the first-contact number of its address", creator.accountNumbers[newcomer] == FirstContactAccountNumber(sdk.MustAccAddressFromBech32(newcomer)))
	_, err = run.server.RegisterAccount(run.ctx, registerAccount)
	run.accepted("Unknown address, first tx RegisterAccount, is stored", err)

	err = ante(run.ctx, signedBy("stranger-anchor", 0, anchor))
	run.refused("Unknown address, first tx Anchor, is refused", err, ErrUnauthorized)
	run.check("Refused first contact creates no account", !creator.created[chainAddress("stranger-anchor")] && !reachedNext)
	err = ante(run.ctx, signedBy("stranger-mixed", 0, &MsgRegisterAccount{Submitter: chainAddress("stranger-mixed")}, anchor))
	run.refused("Unknown address, RegisterAccount with an Anchor in the same tx, is refused", err, ErrUnauthorized)
	err = ante(run.ctx, signedBy("stranger-sequence", 1, &MsgRegisterAccount{Submitter: chainAddress("stranger-sequence")}))
	run.refused("Unknown address with sequence 1 is refused", err, ErrUnauthorized)
	err = ante(run.ctx, signedBy("known", 3, anchor))
	run.accepted("Known address, any message, passes through", err)
	run.check("Known address reaches the next decorator", reachedNext)

	capCtx := run.ctx.WithBlockHeight(30)
	createdInCapBlock := 0
	for newAccountNumber := 1; newAccountNumber <= MaximumAccountsCreatedPerBlock; newAccountNumber++ {
		seed := fmt.Sprintf("cap-%d", newAccountNumber)
		if ante(capCtx, signedBy(seed, 0, &MsgRegisterAccount{Submitter: chainAddress(seed)})) == nil && creator.created[chainAddress(seed)] {
			createdInCapBlock++
		}
	}
	run.check("Twenty new accounts in one block pass", createdInCapBlock == MaximumAccountsCreatedPerBlock)
	err = ante(capCtx, signedBy("cap-21", 0, &MsgRegisterAccount{Submitter: chainAddress("cap-21")}))
	run.refused("The 21st new account in a block is refused with account_cap", err, ErrAccountCap)
	run.check("The refused 21st account is not created", !creator.created[chainAddress("cap-21")])
	err = ante(run.ctx.WithBlockHeight(31), signedBy("cap-21", 0, &MsgRegisterAccount{Submitter: chainAddress("cap-21")}))
	run.accepted("The 21st account passes in the next block", err)

	authStoreKey, networkStoreKey := storetypes.NewKVStoreKey(authtypes.StoreKey), storetypes.NewKVStoreKey(StoreKey)
	authCtx := testutil.DefaultContextWithKeys(map[string]*storetypes.KVStoreKey{authtypes.StoreKey: authStoreKey, StoreKey: networkStoreKey}, nil, nil).
		WithChainID("traderswift-1").WithBlockHeight(40).WithBlockTime(time.Unix(1_800_000_000, 0))
	authEncoding := moduletestutil.MakeTestEncodingConfig(auth.AppModuleBasic{})
	realAuthKeeper := authkeeper.NewAccountKeeper(authEncoding.Codec, runtime.NewKVStoreService(authStoreKey), authtypes.ProtoBaseAccount,
		map[string][]string{authtypes.FeeCollectorName: nil}, authcodec.NewBech32Codec("cosmos"), "cosmos", chainAddress("authority"))
	collider := sdk.MustAccAddressFromBech32(chainAddress("collider"))
	holder := sdk.MustAccAddressFromBech32(chainAddress("holder"))
	realAuthKeeper.InitGenesis(authCtx, *authtypes.NewGenesisState(authtypes.DefaultParams(), authtypes.GenesisAccounts{authtypes.NewBaseAccount(holder, nil, FirstContactAccountNumber(collider), 0)}))
	realAccounts := AuthChainAccounts{AccountKeeper: realAuthKeeper}
	realDecorator := NewFirstContactDecorator(NewKeeper(authEncoding.Codec, runtime.NewKVStoreService(networkStoreKey), fixedAdminSource{admin: admin}, realAccounts))
	_, err = realDecorator.AnteHandle(authCtx, signedBy("collider", 0, &MsgRegisterAccount{Submitter: collider.String()}), false, next)
	run.refused("First contact whose derived number a genesis account holds is refused with account_number_taken", err, ErrNumberTaken)
	run.check("No account is created for the refused address", !realAuthKeeper.HasAccount(authCtx, collider))
	_, err = realDecorator.AnteHandle(authCtx, signedBy("newcomer", 0, registerAccount), false, next)
	run.accepted("First contact through the real auth keeper", err)
	createdAccount := realAuthKeeper.GetAccount(authCtx, sdk.MustAccAddressFromBech32(newcomer))
	run.check("Real auth account holds the derived number", createdAccount != nil && createdAccount.GetAccountNumber() == FirstContactAccountNumber(sdk.MustAccAddressFromBech32(newcomer)))

	existingBody := FirstContactSigningBody(holder, realAuthKeeper.GetAccount(authCtx, holder))
	run.check("first_contact for an existing account returns its real number and sequence", existingBody["exists"] == true &&
		existingBody["account_number"] == fmt.Sprint(FirstContactAccountNumber(collider)) && existingBody["sequence"] == uint64(0))
	createdAccount.SetSequence(4)
	sequencedBody := FirstContactSigningBody(sdk.MustAccAddressFromBech32(newcomer), createdAccount)
	run.check("first_contact for an existing account returns its real sequence", sequencedBody["sequence"] == uint64(4))
	unknownAddress := sdk.MustAccAddressFromBech32(chainAddress("nobody-yet"))
	unknownBody := FirstContactSigningBody(unknownAddress, nil)
	run.check("first_contact for an unknown address returns the derived number and sequence 0", unknownBody["exists"] == false &&
		unknownBody["account_number"] == fmt.Sprint(FirstContactAccountNumber(unknownAddress)) && unknownBody["sequence"] == uint64(0))

	t.Logf("first contact scenarios: %d of %d passed", run.passed, run.passed+run.failed)
	fmt.Printf("first contact scenarios: %d of %d passed\n", run.passed, run.passed+run.failed)
}
