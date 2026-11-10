package network

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txsigning "github.com/cosmos/cosmos-sdk/types/tx/signing"
)

const MaximumAccountsCreatedPerBlock = 20

const firstContactAccountNumberDomain = "traderswift first contact account number "

type firstContactTx interface {
	sdk.Tx
	GetSigners() ([][]byte, error)
	GetSignaturesV2() ([]txsigning.SignatureV2, error)
}

type FirstContactDecorator struct {
	keeper *Keeper
}

func NewFirstContactDecorator(keeper *Keeper) FirstContactDecorator {
	return FirstContactDecorator{keeper: keeper}
}

func FirstContactAccountNumber(address sdk.AccAddress) uint64 {
	digest := sha256.Sum256(append([]byte(firstContactAccountNumberDomain), address...))
	return binary.BigEndian.Uint64(digest[:8]) | uint64(1)<<63
}

func isFirstContactMessage(message sdk.Msg) bool {
	switch message.(type) {
	case *MsgRegisterAccount, *MsgClaimOwner, *MsgRegisterProvider:
		return true
	}
	return false
}

func everyMessageIsFirstContact(messages []sdk.Msg) bool {
	if len(messages) == 0 {
		return false
	}
	for _, message := range messages {
		if !isFirstContactMessage(message) {
			return false
		}
	}
	return true
}

func (keeper *Keeper) accountsCreatedInBlock(ctx sdk.Context) (uint64, error) {
	createdAt, err := keeper.AccountsCreatedAt.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if createdAt.K1() != ctx.BlockHeight() {
		return 0, nil
	}
	return createdAt.K2(), nil
}

func (decorator FirstContactDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	signedTx, ok := tx.(firstContactTx)
	if !ok {
		return next(ctx, tx, simulate)
	}
	signers, err := signedTx.GetSigners()
	if err != nil {
		return ctx, err
	}
	signatures, err := signedTx.GetSignaturesV2()
	if err != nil {
		return ctx, err
	}
	unknownSigners := []sdk.AccAddress{}
	for signerIndex, signer := range signers {
		address := sdk.AccAddress(signer)
		if decorator.keeper.chainAccountCreator.HasAccount(ctx, address) {
			continue
		}
		if !everyMessageIsFirstContact(tx.GetMsgs()) {
			return ctx, errorsmod.Wrapf(ErrUnauthorized, "%s is not a chain account; its first transaction may hold only RegisterAccount, ClaimOwner or RegisterProvider", address)
		}
		if signerIndex >= len(signatures) || signatures[signerIndex].Sequence != 0 {
			return ctx, errorsmod.Wrapf(ErrUnauthorized, "%s is not a chain account; its first transaction must have sequence 0", address)
		}
		unknownSigners = append(unknownSigners, address)
	}
	if len(unknownSigners) == 0 {
		return next(ctx, tx, simulate)
	}
	createdInBlock, err := decorator.keeper.accountsCreatedInBlock(ctx)
	if err != nil {
		return ctx, err
	}
	if createdInBlock+uint64(len(unknownSigners)) > MaximumAccountsCreatedPerBlock {
		return ctx, errorsmod.Wrapf(ErrAccountCap, "%d new accounts already created in block %d; resend in the next block", createdInBlock, ctx.BlockHeight())
	}
	for _, address := range unknownSigners {
		taken, err := decorator.keeper.chainAccountCreator.HasAccountNumber(ctx, FirstContactAccountNumber(address))
		if err != nil {
			return ctx, err
		}
		if taken {
			return ctx, errorsmod.Wrapf(ErrNumberTaken, "account number %d of %s is held by another account", FirstContactAccountNumber(address), address)
		}
	}
	for _, address := range unknownSigners {
		account := decorator.keeper.chainAccountCreator.NewAccountWithAddress(ctx, address)
		if err := account.SetAccountNumber(FirstContactAccountNumber(address)); err != nil {
			return ctx, err
		}
		decorator.keeper.chainAccountCreator.SetAccount(ctx, account)
	}
	if err := decorator.keeper.AccountsCreatedAt.Set(ctx, collections.Join(ctx.BlockHeight(), createdInBlock+uint64(len(unknownSigners)))); err != nil {
		return ctx, err
	}
	return next(ctx, tx, simulate)
}
