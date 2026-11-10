package network

import (
	"context"
	"strconv"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"traderswift-chain/identitylogic"
)

func (keeper *Keeper) endorserIdOfSubmitter(ctx context.Context, charter Charter, submitter string) (string, error) {
	providerId, err := keeper.ProviderIdByAddress.Get(ctx, collections.Join(charter.Subnet, submitter))
	if err == nil {
		provider, err := keeper.Providers.Get(ctx, collections.Join(charter.Subnet, providerId))
		if err != nil {
			return "", err
		}
		vetoed, err := keeper.isVetoed(ctx, charter.Subnet, identitylogic.TargetKindProvider, providerId)
		if err != nil {
			return "", err
		}
		if provider.EndorsedHeight > 0 && !vetoed {
			return providerId, nil
		}
	} else if !errorsmod.IsOf(err, collections.ErrNotFound) {
		return "", err
	}
	if identitylogic.Contains(charter.Operators, submitter) {
		return submitter, nil
	}
	return "", errorsmod.Wrapf(ErrUnauthorized, "%s is neither an endorsed provider nor an operator of %s", submitter, charter.Subnet)
}

func (server *msgServer) EndorseProvider(ctx context.Context, msg *MsgEndorseProvider) (*MsgEndorseProviderResponse, error) {
	if err := identitylogic.ValidateCompactName("subnet", msg.Subnet); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateCompactName("provider_id", msg.ProviderId); err != nil {
		return nil, invalidField(err)
	}
	charter, err := server.keeper.requireLiveCharter(ctx, msg.Subnet)
	if err != nil {
		return nil, err
	}
	endorserId, err := server.keeper.endorserIdOfSubmitter(ctx, charter, msg.Submitter)
	if err != nil {
		return nil, err
	}
	providerKey := collections.Join(msg.Subnet, msg.ProviderId)
	provider, err := server.keeper.Providers.Get(ctx, providerKey)
	if err != nil {
		return nil, errorsmod.Wrapf(ErrNotFound, "provider %s in %s", msg.ProviderId, msg.Subnet)
	}
	if vetoed, err := server.keeper.isVetoed(ctx, msg.Subnet, identitylogic.TargetKindProvider, msg.ProviderId); err != nil {
		return nil, err
	} else if vetoed {
		return nil, errorsmod.Wrapf(ErrVetoed, "provider %s in %s", msg.ProviderId, msg.Subnet)
	}
	endorsementKey := collections.Join3(msg.Subnet, msg.ProviderId, endorserId)
	if exists, err := server.keeper.Endorsements.Has(ctx, endorsementKey); err != nil {
		return nil, err
	} else if exists {
		return nil, errorsmod.Wrapf(ErrAlreadyExists, "endorsement of %s by %s in %s", msg.ProviderId, endorserId, msg.Subnet)
	}
	height := uint64(sdk.UnwrapSDKContext(ctx).BlockHeight())
	endorsement := Endorsement{Subnet: msg.Subnet, ProviderId: msg.ProviderId, EndorserId: endorserId, Height: height}
	if err := server.keeper.Endorsements.Set(ctx, endorsementKey, endorsement); err != nil {
		return nil, err
	}
	if provider.EndorsedHeight == 0 {
		provider.EndorsedHeight = height
		if err := server.keeper.Providers.Set(ctx, providerKey, provider); err != nil {
			return nil, err
		}
	}
	emitNetworkEvent(ctx, "provider_endorsed", sdk.NewAttribute("subnet", msg.Subnet), sdk.NewAttribute("provider_id", msg.ProviderId), sdk.NewAttribute("endorser_id", endorserId), sdk.NewAttribute("endorsed_height", strconv.FormatUint(provider.EndorsedHeight, 10)))
	return &MsgEndorseProviderResponse{}, nil
}
