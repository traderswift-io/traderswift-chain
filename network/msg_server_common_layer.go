package network

import (
	"context"
	"strconv"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"traderswift-chain/identitylogic"
)

func (server *msgServer) SetCharter(ctx context.Context, msg *MsgSetCharter) (*MsgSetCharterResponse, error) {
	if err := server.keeper.requireValidatorAdmin(ctx, msg.Submitter); err != nil {
		return nil, err
	}
	if err := identitylogic.ValidateCompactName("subnet", msg.Subnet); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateHashHex("charter_hash", msg.CharterHash); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateKindList("provider_kinds", msg.ProviderKinds); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateKindList("segment_kinds", msg.SegmentKinds); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateOperatorList(msg.Operators); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateJoinRule(msg.Join); err != nil {
		return nil, invalidField(err)
	}
	storedVersion, err := server.keeper.CurrentCharterVersion.Get(ctx, msg.Subnet)
	if err != nil && !errorsmod.IsOf(err, collections.ErrNotFound) {
		return nil, err
	}
	if msg.Version != storedVersion+1 {
		return nil, errorsmod.Wrapf(ErrInvalidField, "version must be %d, got %d", storedVersion+1, msg.Version)
	}
	charter := Charter{
		Subnet:        msg.Subnet,
		Version:       msg.Version,
		CharterHash:   msg.CharterHash,
		ProviderKinds: msg.ProviderKinds,
		SegmentKinds:  msg.SegmentKinds,
		Operators:     msg.Operators,
		Height:        uint64(sdk.UnwrapSDKContext(ctx).BlockHeight()),
		Join:          msg.Join,
	}
	if err := server.keeper.Charters.Set(ctx, collections.Join(msg.Subnet, msg.Version), charter); err != nil {
		return nil, err
	}
	if err := server.keeper.CurrentCharterVersion.Set(ctx, msg.Subnet, msg.Version); err != nil {
		return nil, err
	}
	emitNetworkEvent(ctx, "charter_set", sdk.NewAttribute("subnet", msg.Subnet), sdk.NewAttribute("version", strconv.FormatUint(msg.Version, 10)), sdk.NewAttribute("charter_hash", msg.CharterHash), sdk.NewAttribute("join", msg.Join))
	return &MsgSetCharterResponse{}, nil
}

func (server *msgServer) RegisterProvider(ctx context.Context, msg *MsgRegisterProvider) (*MsgRegisterProviderResponse, error) {
	if err := identitylogic.ValidateCompactName("subnet", msg.Subnet); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateCompactName("provider_id", msg.ProviderId); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateCompactName("provider_kind", msg.ProviderKind); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateChainAddress("provider_address", msg.ProviderAddress); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateIdentifier("account_id", msg.AccountId); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateEndpoint(msg.Endpoint); err != nil {
		return nil, invalidField(err)
	}
	charter, err := server.keeper.requireLiveCharter(ctx, msg.Subnet)
	if err != nil {
		return nil, err
	}
	if msg.Submitter != msg.ProviderAddress && !identitylogic.Contains(charter.Operators, msg.Submitter) {
		return nil, errorsmod.Wrapf(ErrUnauthorized, "%s is neither the provider address nor an operator of %s", msg.Submitter, msg.Subnet)
	}
	if !identitylogic.Contains(charter.ProviderKinds, msg.ProviderKind) {
		return nil, errorsmod.Wrapf(ErrInvalidField, "provider kind %s is not in the %s charter", msg.ProviderKind, msg.Subnet)
	}
	ownerAddress, err := server.keeper.requireOwnerAddressOfAccount(ctx, msg.AccountId)
	if err != nil {
		return nil, err
	}
	registrationText := identitylogic.ProviderRegistrationText(identitylogic.ProviderRegistration{
		ChainId:         sdk.UnwrapSDKContext(ctx).ChainID(),
		Subnet:          msg.Subnet,
		ProviderId:      msg.ProviderId,
		ProviderKind:    msg.ProviderKind,
		ProviderAddress: msg.ProviderAddress,
		Endpoint:        msg.Endpoint,
	})
	if err := identitylogic.VerifyEip191Owner(registrationText, msg.OwnerSignature, ownerAddress); err != nil {
		return nil, refusedOwnerSignature(err)
	}
	if exists, err := server.keeper.Providers.Has(ctx, collections.Join(msg.Subnet, msg.ProviderId)); err != nil {
		return nil, err
	} else if exists {
		return nil, errorsmod.Wrapf(ErrAlreadyExists, "provider %s in %s", msg.ProviderId, msg.Subnet)
	}
	if exists, err := server.keeper.ProviderIdByAddress.Has(ctx, collections.Join(msg.Subnet, msg.ProviderAddress)); err != nil {
		return nil, err
	} else if exists {
		return nil, errorsmod.Wrapf(ErrAlreadyExists, "provider address %s in %s", msg.ProviderAddress, msg.Subnet)
	}
	registrationHeight := uint64(sdk.UnwrapSDKContext(ctx).BlockHeight())
	provider := Provider{
		Subnet:          msg.Subnet,
		ProviderId:      msg.ProviderId,
		ProviderKind:    msg.ProviderKind,
		ProviderAddress: msg.ProviderAddress,
		AccountId:       msg.AccountId,
		Endpoint:        msg.Endpoint,
		Height:          registrationHeight,
	}
	if charter.Join == identitylogic.JoinOpen {
		provider.EndorsedHeight = registrationHeight
	}
	if err := server.keeper.Providers.Set(ctx, collections.Join(msg.Subnet, msg.ProviderId), provider); err != nil {
		return nil, err
	}
	if err := server.keeper.ProviderIdByAddress.Set(ctx, collections.Join(msg.Subnet, msg.ProviderAddress), msg.ProviderId); err != nil {
		return nil, err
	}
	if err := server.keeper.ensureChainAccountExists(ctx, msg.ProviderAddress); err != nil {
		return nil, err
	}
	emitNetworkEvent(ctx, "provider_registered", sdk.NewAttribute("subnet", msg.Subnet), sdk.NewAttribute("provider_id", msg.ProviderId), sdk.NewAttribute("provider_kind", msg.ProviderKind), sdk.NewAttribute("endorsed", strconv.FormatBool(provider.EndorsedHeight > 0)))
	return &MsgRegisterProviderResponse{}, nil
}

func splitAnchorVetoTargetId(targetId string) (string, string, string, bool) {
	firstColon := strings.Index(targetId, ":")
	lastColon := strings.LastIndex(targetId, ":")
	if firstColon < 0 || lastColon == firstColon {
		return "", "", "", false
	}
	return targetId[:firstColon], targetId[firstColon+1 : lastColon], targetId[lastColon+1:], true
}

func (server *msgServer) Veto(ctx context.Context, msg *MsgVeto) (*MsgVetoResponse, error) {
	if err := server.keeper.requireValidatorAdmin(ctx, msg.Submitter); err != nil {
		return nil, err
	}
	if err := identitylogic.ValidateCompactName("subnet", msg.Subnet); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateVetoReason(msg.ReasonCode, msg.Reason); err != nil {
		return nil, invalidField(err)
	}
	var vetoedAnchor *Anchor
	switch msg.TargetKind {
	case identitylogic.TargetKindCharter:
		version, err := strconv.ParseUint(msg.TargetId, 10, 64)
		if err != nil || CharterVetoTargetId(version) != msg.TargetId {
			return nil, errorsmod.Wrap(ErrInvalidField, "a charter veto target_id is the version number")
		}
		if exists, err := server.keeper.Charters.Has(ctx, collections.Join(msg.Subnet, version)); err != nil {
			return nil, err
		} else if !exists {
			return nil, errorsmod.Wrapf(ErrNotFound, "charter %s version %d", msg.Subnet, version)
		}
	case identitylogic.TargetKindProvider:
		if exists, err := server.keeper.Providers.Has(ctx, collections.Join(msg.Subnet, msg.TargetId)); err != nil {
			return nil, err
		} else if !exists {
			return nil, errorsmod.Wrapf(ErrNotFound, "provider %s in %s", msg.TargetId, msg.Subnet)
		}
	case identitylogic.TargetKindAnchor:
		segmentKind, segmentId, providerId, parsed := splitAnchorVetoTargetId(msg.TargetId)
		if !parsed {
			return nil, errorsmod.Wrap(ErrInvalidField, "an anchor veto target_id is segment_kind:segment_id:provider_id")
		}
		anchor, err := server.keeper.Anchors.Get(ctx, anchorKeyOf(msg.Subnet, segmentKind, segmentId, providerId))
		if err != nil {
			return nil, errorsmod.Wrapf(ErrNotFound, "anchor %s in %s", msg.TargetId, msg.Subnet)
		}
		vetoedAnchor = &anchor
	default:
		return nil, errorsmod.Wrap(ErrInvalidField, "target_kind must be charter, provider or anchor")
	}
	vetoKey := collections.Join3(msg.Subnet, msg.TargetKind, msg.TargetId)
	if exists, err := server.keeper.Vetoes.Has(ctx, vetoKey); err != nil {
		return nil, err
	} else if exists {
		return nil, errorsmod.Wrapf(ErrAlreadyExists, "veto on %s %s in %s", msg.TargetKind, msg.TargetId, msg.Subnet)
	}
	veto := Veto{
		Subnet:     msg.Subnet,
		TargetKind: msg.TargetKind,
		TargetId:   msg.TargetId,
		ReasonCode: msg.ReasonCode,
		Reason:     msg.Reason,
		Height:     uint64(sdk.UnwrapSDKContext(ctx).BlockHeight()),
	}
	if err := server.keeper.Vetoes.Set(ctx, vetoKey, veto); err != nil {
		return nil, err
	}
	if vetoedAnchor != nil {
		vetoedAnchor.Vetoed = true
		if err := server.keeper.Anchors.Set(ctx, anchorKeyOf(vetoedAnchor.Subnet, vetoedAnchor.SegmentKind, vetoedAnchor.SegmentId, vetoedAnchor.ProviderId), *vetoedAnchor); err != nil {
			return nil, err
		}
	}
	emitNetworkEvent(ctx, "vetoed", sdk.NewAttribute("subnet", msg.Subnet), sdk.NewAttribute("target_kind", msg.TargetKind), sdk.NewAttribute("target_id", msg.TargetId), sdk.NewAttribute("reason_code", msg.ReasonCode))
	return &MsgVetoResponse{}, nil
}

func (server *msgServer) Anchor(ctx context.Context, msg *MsgAnchor) (*MsgAnchorResponse, error) {
	if err := identitylogic.ValidateCompactName("subnet", msg.Subnet); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateCompactName("segment_kind", msg.SegmentKind); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateIdentifier("segment_id", msg.SegmentId); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateHashHex("segment_hash", msg.SegmentHash); err != nil {
		return nil, invalidField(err)
	}
	if err := identitylogic.ValidateCompactName("provider_id", msg.ProviderId); err != nil {
		return nil, invalidField(err)
	}
	charter, err := server.keeper.requireLiveCharter(ctx, msg.Subnet)
	if err != nil {
		return nil, err
	}
	if !identitylogic.Contains(charter.SegmentKinds, msg.SegmentKind) {
		return nil, errorsmod.Wrapf(ErrInvalidField, "segment kind %s is not in the %s charter", msg.SegmentKind, msg.Subnet)
	}
	provider, err := server.keeper.Providers.Get(ctx, collections.Join(msg.Subnet, msg.ProviderId))
	if err != nil {
		return nil, errorsmod.Wrapf(ErrNotFound, "provider %s in %s", msg.ProviderId, msg.Subnet)
	}
	if vetoed, err := server.keeper.isVetoed(ctx, msg.Subnet, identitylogic.TargetKindProvider, msg.ProviderId); err != nil {
		return nil, err
	} else if vetoed {
		return nil, errorsmod.Wrapf(ErrVetoed, "provider %s in %s", msg.ProviderId, msg.Subnet)
	}
	if provider.ProviderAddress != msg.Submitter {
		return nil, errorsmod.Wrapf(ErrUnauthorized, "%s is not the address of provider %s", msg.Submitter, msg.ProviderId)
	}
	anchorKey := anchorKeyOf(msg.Subnet, msg.SegmentKind, msg.SegmentId, msg.ProviderId)
	if exists, err := server.keeper.Anchors.Has(ctx, anchorKey); err != nil {
		return nil, err
	} else if exists {
		return nil, errorsmod.Wrapf(ErrAlreadyExists, "anchor %s", AnchorVetoTargetId(msg.SegmentKind, msg.SegmentId, msg.ProviderId))
	}
	anchor := Anchor{
		Subnet:      msg.Subnet,
		SegmentKind: msg.SegmentKind,
		SegmentId:   msg.SegmentId,
		ProviderId:  msg.ProviderId,
		SegmentHash: msg.SegmentHash,
		Height:      uint64(sdk.UnwrapSDKContext(ctx).BlockHeight()),
		Endorsed:    provider.EndorsedHeight > 0,
	}
	if err := server.keeper.Anchors.Set(ctx, anchorKey, anchor); err != nil {
		return nil, err
	}
	emitNetworkEvent(ctx, "anchored", sdk.NewAttribute("subnet", msg.Subnet), sdk.NewAttribute("segment_kind", msg.SegmentKind), sdk.NewAttribute("segment_id", msg.SegmentId), sdk.NewAttribute("provider_id", msg.ProviderId), sdk.NewAttribute("segment_hash", msg.SegmentHash), sdk.NewAttribute("endorsed", strconv.FormatBool(anchor.Endorsed)))
	return &MsgAnchorResponse{}, nil
}
