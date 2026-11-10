package network

import (
	"context"
	"strconv"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"traderswift-chain/identitylogic"
)

type msgServer struct {
	keeper *Keeper
}

func NewMsgServer(keeper *Keeper) MsgServer {
	return &msgServer{keeper: keeper}
}

func invalidField(err error) error {
	return errorsmod.Wrap(ErrInvalidField, err.Error())
}

func refusedOwnerSignature(err error) error {
	return errorsmod.Wrap(ErrUnauthorized, err.Error())
}

func blockTimeSeconds(ctx context.Context) uint64 {
	return uint64(sdk.UnwrapSDKContext(ctx).BlockTime().Unix())
}

func (server *msgServer) RegisterAccount(ctx context.Context, msg *MsgRegisterAccount) (*MsgRegisterAccountResponse, error) {
	if err := identitylogic.ValidateIdentifier("account_id", msg.AccountId); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidatePublicKeyHex("registration_key", msg.RegistrationKey); err != nil {
		return nil, invalidField(err)
	}
	if msg.Expiry <= blockTimeSeconds(ctx) {
		return nil, errorsmod.Wrap(ErrInvalidField, "expiry must be in the future")
	}
	existing, err := server.keeper.Accounts.Get(ctx, msg.AccountId)
	if err == nil {
		unclaimedAndExpired := existing.OwnerAddress == "" && existing.Expiry <= blockTimeSeconds(ctx)
		if !unclaimedAndExpired {
			return nil, errorsmod.Wrapf(ErrAlreadyExists, "account %s", msg.AccountId)
		}
	}
	account := Account{AccountId: msg.AccountId, RegistrationKey: msg.RegistrationKey, Expiry: msg.Expiry}
	if err := server.keeper.Accounts.Set(ctx, msg.AccountId, account); err != nil {
		return nil, err
	}
	emitNetworkEvent(ctx, "account_registered", sdk.NewAttribute("account_id", msg.AccountId), sdk.NewAttribute("expiry", strconv.FormatUint(msg.Expiry, 10)))
	return &MsgRegisterAccountResponse{}, nil
}

func (server *msgServer) ClaimOwner(ctx context.Context, msg *MsgClaimOwner) (*MsgClaimOwnerResponse, error) {
	account, err := server.keeper.Accounts.Get(ctx, msg.AccountId)
	if err != nil {
		return nil, errorsmod.Wrapf(ErrNotFound, "account %s", msg.AccountId)
	}
	if account.OwnerAddress != "" {
		return nil, errorsmod.Wrapf(ErrAlreadyExists, "account %s already has an owner", msg.AccountId)
	}
	if account.Expiry <= blockTimeSeconds(ctx) {
		return nil, errorsmod.Wrapf(ErrExpired, "account %s", msg.AccountId)
	}
	claim := identitylogic.OwnerClaim{
		ChainId:         sdk.UnwrapSDKContext(ctx).ChainID(),
		AccountId:       msg.AccountId,
		RegistrationKey: account.RegistrationKey,
		OwnerAddress:    msg.OwnerAddress,
		Nonce:           msg.Nonce,
		IssuedAt:        msg.IssuedAt,
		ExpiresAt:       msg.ExpiresAt,
	}
	if err := identitylogic.ValidateOwnerClaimWindow(claim, blockTimeSeconds(ctx)); err != nil {
		return nil, invalidField(err)
	}
	claimText, err := identitylogic.OwnerClaimText(claim)
	if err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.VerifyEip191Owner(claimText, msg.OwnerSignature, msg.OwnerAddress); err != nil {
		return nil, refusedOwnerSignature(err)
	}
	if err := identitylogic.VerifyEd25519Signature(account.RegistrationKey, []byte(claimText), msg.RegistrationSignature); err != nil {
		return nil, refusedOwnerSignature(err)
	}
	account.OwnerAddress = msg.OwnerAddress
	if err := server.keeper.Accounts.Set(ctx, msg.AccountId, account); err != nil {
		return nil, err
	}
	emitNetworkEvent(ctx, "claimed", sdk.NewAttribute("account_id", msg.AccountId), sdk.NewAttribute("owner_address", msg.OwnerAddress))
	return &MsgClaimOwnerResponse{}, nil
}
