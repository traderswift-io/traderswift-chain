package network

import (
	"context"
	"errors"

	"cosmossdk.io/collections"

	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
)

type AuthChainAccounts struct {
	authkeeper.AccountKeeper
}

func (accounts AuthChainAccounts) HasAccountNumber(ctx context.Context, accountNumber uint64) (bool, error) {
	_, err := accounts.Accounts.Indexes.Number.MatchExact(ctx, accountNumber)
	if errors.Is(err, collections.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

var _ ChainAccountCreator = AuthChainAccounts{}
