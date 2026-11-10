package network

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"traderswift-chain/identitylogic"
)

func DefaultGenesisState() *GenesisState {
	return &GenesisState{}
}

func validateGenesisAccounts(state *GenesisState) (map[string]Account, error) {
	accountsById := map[string]Account{}
	for _, account := range state.Accounts {
		if err := identitylogic.ValidateIdentifier("account_id", account.AccountId); err != nil {
			return nil, err
		}
		if err := identitylogic.ValidatePublicKeyHex("registration_key", account.RegistrationKey); err != nil {
			return nil, err
		}
		if account.OwnerAddress != "" {
			if err := identitylogic.ValidateEthereumAddress("owner_address", account.OwnerAddress); err != nil {
				return nil, err
			}
		}
		if _, seen := accountsById[account.AccountId]; seen {
			return nil, fmt.Errorf("account %s twice in genesis", account.AccountId)
		}
		accountsById[account.AccountId] = *account
	}
	return accountsById, nil
}

func validateGenesisCharters(state *GenesisState) (map[string]Charter, error) {
	latestCharterBySubnet := map[string]Charter{}
	for _, charter := range state.Charters {
		if err := identitylogic.ValidateCompactName("subnet", charter.Subnet); err != nil {
			return nil, err
		}
		if err := identitylogic.ValidateHashHex("charter_hash", charter.CharterHash); err != nil {
			return nil, err
		}
		if err := identitylogic.ValidateKindList("provider_kinds", charter.ProviderKinds); err != nil {
			return nil, err
		}
		if err := identitylogic.ValidateKindList("segment_kinds", charter.SegmentKinds); err != nil {
			return nil, err
		}
		if err := identitylogic.ValidateOperatorList(charter.Operators); err != nil {
			return nil, err
		}
		if err := identitylogic.ValidateJoinRule(charter.Join); err != nil {
			return nil, fmt.Errorf("charter %s version %d: %v", charter.Subnet, charter.Version, err)
		}
		previous := latestCharterBySubnet[charter.Subnet]
		if charter.Version != previous.Version+1 {
			return nil, fmt.Errorf("charter %s: versions must run 1, 2, 3 in order, got %d after %d", charter.Subnet, charter.Version, previous.Version)
		}
		latestCharterBySubnet[charter.Subnet] = *charter
	}
	return latestCharterBySubnet, nil
}

func ValidateGenesisState(state *GenesisState) error {
	accountsById, err := validateGenesisAccounts(state)
	if err != nil {
		return err
	}
	latestCharterBySubnet, err := validateGenesisCharters(state)
	if err != nil {
		return err
	}
	providerIds := map[string]bool{}
	providerAddresses := map[string]bool{}
	endorsedHeightByProvider := map[string]uint64{}
	for _, provider := range state.Providers {
		charter, hasCharter := latestCharterBySubnet[provider.Subnet]
		if !hasCharter {
			return fmt.Errorf("provider %s: no charter for subnet %s", provider.ProviderId, provider.Subnet)
		}
		if err := identitylogic.ValidateCompactName("provider_id", provider.ProviderId); err != nil {
			return err
		}
		if !identitylogic.Contains(charter.ProviderKinds, provider.ProviderKind) {
			return fmt.Errorf("provider %s: kind %s is not in the %s charter", provider.ProviderId, provider.ProviderKind, provider.Subnet)
		}
		if err := identitylogic.ValidateChainAddress("provider_address", provider.ProviderAddress); err != nil {
			return err
		}
		if err := identitylogic.ValidateEndpoint(provider.Endpoint); err != nil {
			return err
		}
		if account, hasAccount := accountsById[provider.AccountId]; !hasAccount || account.OwnerAddress == "" {
			return fmt.Errorf("provider %s: account %s missing or not claimed", provider.ProviderId, provider.AccountId)
		}
		idKey, addressKey := provider.Subnet+"/"+provider.ProviderId, provider.Subnet+"/"+provider.ProviderAddress
		if providerIds[idKey] || providerAddresses[addressKey] {
			return fmt.Errorf("provider %s or its address twice in %s", provider.ProviderId, provider.Subnet)
		}
		providerIds[idKey], providerAddresses[addressKey] = true, true
		endorsedHeightByProvider[idKey] = provider.EndorsedHeight
	}
	endorsementKeys := map[string]bool{}
	for _, endorsement := range state.Endorsements {
		idKey := endorsement.Subnet + "/" + endorsement.ProviderId
		if !providerIds[idKey] {
			return fmt.Errorf("endorsement of %s: provider unknown in %s", endorsement.ProviderId, endorsement.Subnet)
		}
		if endorsedHeightByProvider[idKey] == 0 {
			return fmt.Errorf("endorsement of %s in %s: provider has no endorsed_height", endorsement.ProviderId, endorsement.Subnet)
		}
		if endorsement.EndorserId == "" {
			return fmt.Errorf("endorsement of %s in %s: empty endorser_id", endorsement.ProviderId, endorsement.Subnet)
		}
		endorsementKey := idKey + "/" + endorsement.EndorserId
		if endorsementKeys[endorsementKey] {
			return fmt.Errorf("endorsement of %s by %s twice in %s", endorsement.ProviderId, endorsement.EndorserId, endorsement.Subnet)
		}
		endorsementKeys[endorsementKey] = true
	}
	for _, veto := range state.Vetoes {
		if err := identitylogic.ValidateCompactName("subnet", veto.Subnet); err != nil {
			return err
		}
		if err := identitylogic.ValidateVetoReason(veto.ReasonCode, veto.Reason); err != nil {
			return err
		}
	}
	for _, anchor := range state.Anchors {
		if !providerIds[anchor.Subnet+"/"+anchor.ProviderId] {
			return fmt.Errorf("anchor %s: provider %s unknown in %s", anchor.SegmentId, anchor.ProviderId, anchor.Subnet)
		}
		if err := identitylogic.ValidateHashHex("segment_hash", anchor.SegmentHash); err != nil {
			return err
		}
		if err := identitylogic.ValidateIdentifier("segment_id", anchor.SegmentId); err != nil {
			return err
		}
	}
	return nil
}

func (keeper *Keeper) InitializeFromGenesis(ctx context.Context, state *GenesisState) error {
	if err := ValidateGenesisState(state); err != nil {
		return err
	}
	for _, account := range state.Accounts {
		if err := keeper.Accounts.Set(ctx, account.AccountId, *account); err != nil {
			return err
		}
	}
	for _, charter := range state.Charters {
		if err := keeper.Charters.Set(ctx, collections.Join(charter.Subnet, charter.Version), *charter); err != nil {
			return err
		}
		if err := keeper.CurrentCharterVersion.Set(ctx, charter.Subnet, charter.Version); err != nil {
			return err
		}
	}
	for _, provider := range state.Providers {
		if err := keeper.Providers.Set(ctx, collections.Join(provider.Subnet, provider.ProviderId), *provider); err != nil {
			return err
		}
		if err := keeper.ProviderIdByAddress.Set(ctx, collections.Join(provider.Subnet, provider.ProviderAddress), provider.ProviderId); err != nil {
			return err
		}
		if err := keeper.ensureChainAccountExists(ctx, provider.ProviderAddress); err != nil {
			return err
		}
	}
	for _, veto := range state.Vetoes {
		if err := keeper.Vetoes.Set(ctx, collections.Join3(veto.Subnet, veto.TargetKind, veto.TargetId), *veto); err != nil {
			return err
		}
	}
	for _, endorsement := range state.Endorsements {
		if err := keeper.Endorsements.Set(ctx, collections.Join3(endorsement.Subnet, endorsement.ProviderId, endorsement.EndorserId), *endorsement); err != nil {
			return err
		}
	}
	for _, anchor := range state.Anchors {
		if err := keeper.Anchors.Set(ctx, anchorKeyOf(anchor.Subnet, anchor.SegmentKind, anchor.SegmentId, anchor.ProviderId), *anchor); err != nil {
			return err
		}
	}
	return nil
}

func collectAllValues[Key any, Value any](ctx context.Context, values collections.Map[Key, Value]) ([]*Value, error) {
	collected := []*Value{}
	err := values.Walk(ctx, nil, func(_ Key, value Value) (bool, error) {
		collected = append(collected, &value)
		return false, nil
	})
	return collected, err
}

func (keeper *Keeper) ExportToGenesis(ctx context.Context) (*GenesisState, error) {
	state := &GenesisState{}
	var err error
	if state.Accounts, err = collectAllValues(ctx, keeper.Accounts); err != nil {
		return nil, err
	}
	if state.Charters, err = collectAllValues(ctx, keeper.Charters); err != nil {
		return nil, err
	}
	if state.Providers, err = collectAllValues(ctx, keeper.Providers); err != nil {
		return nil, err
	}
	if state.Vetoes, err = collectAllValues(ctx, keeper.Vetoes); err != nil {
		return nil, err
	}
	if state.Anchors, err = collectAllValues(ctx, keeper.Anchors); err != nil {
		return nil, err
	}
	if state.Endorsements, err = collectAllValues(ctx, keeper.Endorsements); err != nil {
		return nil, err
	}
	return state, nil
}
