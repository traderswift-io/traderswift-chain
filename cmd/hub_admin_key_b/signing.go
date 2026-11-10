package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cosmos/gogoproto/proto"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/cosmos/cosmos-sdk/x/tx/signing"

	"traderswift-chain/network"
	"traderswift-chain/validatorset"
)

type signRequest struct {
	RequestId     string          `json:"request_id"`
	UnsignedTx    json.RawMessage `json:"unsigned_tx"`
	AccountNumber uint64          `json:"account_number,string"`
	Sequence      uint64          `json:"sequence,string"`
}

func newTxConfig() (client.TxConfig, error) {
	registry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec:          address.Bech32Codec{Bech32Prefix: sdk.GetConfig().GetBech32AccountAddrPrefix()},
			ValidatorAddressCodec: address.Bech32Codec{Bech32Prefix: sdk.GetConfig().GetBech32ValidatorAddrPrefix()},
		},
	})
	if err != nil {
		return nil, err
	}
	std.RegisterInterfaces(registry)
	network.AppModule{}.RegisterInterfaces(registry)
	validatorset.AppModule{}.RegisterInterfaces(registry)
	return authtx.NewTxConfigWithOptions(codec.NewProtoCodec(registry), authtx.ConfigOptions{})
}

func refusal(format string, arguments ...any) error {
	return fmt.Errorf("refused: "+format, arguments...)
}

func checkedRoutineTx(txConfig client.TxConfig, unsignedTxJson []byte, adminAddress string) (authsigning.Tx, []string, error) {
	decoded, err := txConfig.TxJSONDecoder()(unsignedTxJson)
	if err != nil {
		return nil, nil, refusal("unsigned_tx does not decode: %v", err)
	}
	transaction, ok := decoded.(authsigning.Tx)
	if !ok {
		return nil, nil, refusal("unsigned_tx is not a signable tx")
	}
	messages := transaction.GetMsgs()
	if len(messages) == 0 {
		return nil, nil, refusal("unsigned_tx has no messages")
	}
	feePayer := sdk.AccAddress(transaction.FeePayer())
	if !transaction.GetFee().IsZero() || len(transaction.FeeGranter()) > 0 || len(feePayer) > 0 && feePayer.String() != adminAddress {
		return nil, nil, refusal("unsigned_tx carries a fee, a fee payer or a fee granter")
	}
	typeUrls := make([]string, 0, len(messages))
	for _, message := range messages {
		typeUrl := sdk.MsgTypeURL(message)
		signer, routine := routineMessageAdminSigner(message)
		if !routine {
			return nil, nil, refusal("%s is not on the routine allowlist", typeUrl)
		}
		if signer != adminAddress {
			return nil, nil, refusal("%s is signed by %s, not the network admin %s", typeUrl, signer, adminAddress)
		}
		typeUrls = append(typeUrls, typeUrl)
	}
	return transaction, typeUrls, nil
}

func aminoJsonSignBytes(txConfig client.TxConfig, transaction authsigning.Tx, chainId string, adminAddress string, accountNumber uint64, sequence uint64) ([]byte, error) {
	signerData := authsigning.SignerData{Address: adminAddress, ChainID: chainId, AccountNumber: accountNumber, Sequence: sequence}
	return authsigning.GetSignBytesAdapter(context.Background(), txConfig.SignModeHandler(), signingtypes.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, signerData, transaction)
}

func signatureDescriptorJson(txConfig client.TxConfig, privateKey *secp256k1.PrivKey, signBytes []byte, sequence uint64) ([]byte, error) {
	signature, err := privateKey.Sign(signBytes)
	if err != nil {
		return nil, err
	}
	signatureV2 := signingtypes.SignatureV2{
		PubKey:   privateKey.PubKey(),
		Data:     &signingtypes.SingleSignatureData{SignMode: signingtypes.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, Signature: signature},
		Sequence: sequence,
	}
	return txConfig.MarshalSignatureJSON([]signingtypes.SignatureV2{signatureV2})
}

func readPrivateKeyFile(path string) (*secp256k1.PrivKey, error) {
	information, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if information.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by group or others (mode %o); it must be 0600", path, information.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	keyBytes, err := hex.DecodeString(strings.TrimSpace(string(content)))
	if err != nil || len(keyBytes) != 32 {
		return nil, errors.New("key file does not hold 32 bytes of hex")
	}
	return &secp256k1.PrivKey{Key: keyBytes}, nil
}
