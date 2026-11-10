package main

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"traderswift-chain/network"
	"traderswift-chain/validatorset"
)

func routineMessageAdminSigner(message sdk.Msg) (string, bool) {
	switch typed := message.(type) {
	case *network.MsgEndorseProvider:
		return typed.Submitter, true
	case *network.MsgRegisterProvider:
		return typed.Submitter, true
	case *network.MsgSetCharter:
		return typed.Submitter, true
	case *validatorset.MsgAddValidator:
		if typed.Founding {
			return "", false
		}
		return typed.Admin, true
	}
	return "", false
}
