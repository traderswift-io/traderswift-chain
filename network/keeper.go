package network

import (
	"context"
	"strconv"

	"cosmossdk.io/collections"
	collectionscodec "cosmossdk.io/collections/codec"
	"cosmossdk.io/core/store"
	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"traderswift-chain/identitylogic"
)

const (
	ModuleName = "network"
	StoreKey   = ModuleName
)

var (
	ErrNotFound      = errorsmod.Register(ModuleName, 2, "not found")
	ErrAlreadyExists = errorsmod.Register(ModuleName, 3, "already exists")
	ErrUnauthorized  = errorsmod.Register(ModuleName, 4, "unauthorized")
	ErrInvalidField  = errorsmod.Register(ModuleName, 5, "invalid field")
	ErrNotClaimed    = errorsmod.Register(ModuleName, 6, "account not claimed by an owner")
	ErrExpired       = errorsmod.Register(ModuleName, 7, "account expired before it was claimed")
	ErrVetoed        = errorsmod.Register(ModuleName, 8, "vetoed")
	ErrAccountCap    = errorsmod.Register(ModuleName, 9, "account_cap")
	ErrNumberTaken   = errorsmod.Register(ModuleName, 10, "account_number_taken")
)

type ValidatorAdminSource interface {
	Admin(ctx sdk.Context) (string, error)
}

type ChainAccountCreator interface {
	HasAccount(ctx context.Context, address sdk.AccAddress) bool
	NewAccountWithAddress(ctx context.Context, address sdk.AccAddress) sdk.AccountI
	SetAccount(ctx context.Context, account sdk.AccountI)
	HasAccountNumber(ctx context.Context, accountNumber uint64) (bool, error)
}

type Keeper struct {
	validatorAdminSource  ValidatorAdminSource
	chainAccountCreator   ChainAccountCreator
	Accounts              collections.Map[string, Account]
	CurrentCharterVersion collections.Map[string, uint64]
	Charters              collections.Map[collections.Pair[string, uint64], Charter]
	Providers             collections.Map[collections.Pair[string, string], Provider]
	ProviderIdByAddress   collections.Map[collections.Pair[string, string], string]
	Vetoes                collections.Map[collections.Triple[string, string, string], Veto]
	Anchors               collections.Map[collections.Quad[string, string, string, string], Anchor]
	Endorsements          collections.Map[collections.Triple[string, string, string], Endorsement]
	AccountsCreatedAt     collections.Item[collections.Pair[int64, uint64]]
}

func NewKeeper(cdc codec.BinaryCodec, storeService store.KVStoreService, validatorAdminSource ValidatorAdminSource, chainAccountCreator ChainAccountCreator) *Keeper {
	schemaBuilder := collections.NewSchemaBuilder(storeService)
	keeper := &Keeper{
		validatorAdminSource:  validatorAdminSource,
		chainAccountCreator:   chainAccountCreator,
		Accounts:              collections.NewMap(schemaBuilder, collections.NewPrefix(1), "accounts", collections.StringKey, codec.CollValue[Account](cdc)),
		CurrentCharterVersion: collections.NewMap(schemaBuilder, collections.NewPrefix(2), "current_charter_version", collections.StringKey, collections.Uint64Value),
		Charters:              collections.NewMap(schemaBuilder, collections.NewPrefix(3), "charters", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key), codec.CollValue[Charter](cdc)),
		Providers:             collections.NewMap(schemaBuilder, collections.NewPrefix(4), "providers", collections.PairKeyCodec(collections.StringKey, collections.StringKey), codec.CollValue[Provider](cdc)),
		ProviderIdByAddress:   collections.NewMap(schemaBuilder, collections.NewPrefix(5), "provider_id_by_address", collections.PairKeyCodec(collections.StringKey, collections.StringKey), collections.StringValue),
		Vetoes:                collections.NewMap(schemaBuilder, collections.NewPrefix(6), "vetoes", collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey), codec.CollValue[Veto](cdc)),
		Anchors:               collections.NewMap(schemaBuilder, collections.NewPrefix(7), "anchors", collections.QuadKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey, collections.StringKey), codec.CollValue[Anchor](cdc)),
		Endorsements:          collections.NewMap(schemaBuilder, collections.NewPrefix(8), "endorsements", collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey), codec.CollValue[Endorsement](cdc)),
		AccountsCreatedAt:     collections.NewItem(schemaBuilder, collections.NewPrefix(9), "accounts_created_at_height", collectionscodec.KeyToValueCodec(collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key))),
	}
	if _, err := schemaBuilder.Build(); err != nil {
		panic(err)
	}
	return keeper
}

func anchorKeyOf(subnet string, segmentKind string, segmentId string, providerId string) collections.Quad[string, string, string, string] {
	return collections.Join4(subnet, segmentKind, segmentId, providerId)
}

func AnchorVetoTargetId(segmentKind string, segmentId string, providerId string) string {
	return segmentKind + ":" + segmentId + ":" + providerId
}

func CharterVetoTargetId(version uint64) string {
	return strconv.FormatUint(version, 10)
}

func (keeper *Keeper) requireValidatorAdmin(ctx context.Context, submitter string) error {
	admin, err := keeper.validatorAdminSource.Admin(sdk.UnwrapSDKContext(ctx))
	if err != nil {
		return err
	}
	if admin != submitter {
		return errorsmod.Wrapf(ErrUnauthorized, "%s is not the network admin", submitter)
	}
	return nil
}

func (keeper *Keeper) isVetoed(ctx context.Context, subnet string, targetKind string, targetId string) (bool, error) {
	return keeper.Vetoes.Has(ctx, collections.Join3(subnet, targetKind, targetId))
}

func (keeper *Keeper) currentCharter(ctx context.Context, subnet string) (Charter, error) {
	version, err := keeper.CurrentCharterVersion.Get(ctx, subnet)
	if err != nil {
		return Charter{}, errorsmod.Wrapf(ErrNotFound, "charter for subnet %s", subnet)
	}
	return keeper.Charters.Get(ctx, collections.Join(subnet, version))
}

func (keeper *Keeper) requireLiveCharter(ctx context.Context, subnet string) (Charter, error) {
	charter, err := keeper.currentCharter(ctx, subnet)
	if err != nil {
		return Charter{}, err
	}
	vetoed, err := keeper.isVetoed(ctx, subnet, identitylogic.TargetKindCharter, CharterVetoTargetId(charter.Version))
	if err != nil {
		return Charter{}, err
	}
	if vetoed {
		return Charter{}, errorsmod.Wrapf(ErrVetoed, "charter %s version %d", subnet, charter.Version)
	}
	return charter, nil
}

func (keeper *Keeper) requireOwnerAddressOfAccount(ctx context.Context, accountId string) (string, error) {
	account, err := keeper.Accounts.Get(ctx, accountId)
	if err != nil {
		return "", errorsmod.Wrapf(ErrNotFound, "account %s", accountId)
	}
	if account.OwnerAddress == "" {
		return "", errorsmod.Wrapf(ErrNotClaimed, "account %s", accountId)
	}
	return account.OwnerAddress, nil
}

func (keeper *Keeper) ensureChainAccountExists(ctx context.Context, bech32Address string) error {
	address, err := sdk.AccAddressFromBech32(bech32Address)
	if err != nil {
		return err
	}
	if keeper.chainAccountCreator.HasAccount(ctx, address) {
		return nil
	}
	keeper.chainAccountCreator.SetAccount(ctx, keeper.chainAccountCreator.NewAccountWithAddress(ctx, address))
	return nil
}

func emitNetworkEvent(ctx context.Context, eventType string, attributes ...sdk.Attribute) {
	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent("network."+eventType, attributes...))
}
