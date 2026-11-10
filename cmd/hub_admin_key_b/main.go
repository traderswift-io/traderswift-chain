package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	"traderswift-chain/chainprefix"
)

const maximumRequestBytes = 1 << 20

type daemonConfig struct {
	Listen       string `json:"listen"`
	ChainId      string `json:"chain_id"`
	AdminAddress string `json:"admin_address"`
	KeyFile      string `json:"key_file"`
}

func main() {
	chainprefix.Apply()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: hub_admin_key_b keygen --key-file f | public --key-file f | sign-doc --unsigned-tx f --chain-id c --admin a --account-number n --sequence s | serve --config f")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = runKeygen(os.Args[2:])
	case "public":
		err = runPublic(os.Args[2:])
	case "sign-doc":
		err = runSignDoc(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printPublic(privateKey *secp256k1.PrivKey) error {
	publicKey := privateKey.PubKey()
	return json.NewEncoder(os.Stdout).Encode(map[string]string{
		"address":           sdk.AccAddress(publicKey.Address()).String(),
		"public_key_base64": base64.StdEncoding.EncodeToString(publicKey.Bytes()),
	})
}

func runKeygen(arguments []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	keyFile := flags.String("key-file", "", "where key B is written, 0600; never overwritten")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *keyFile == "" {
		return errors.New("--key-file is required")
	}
	handle, err := os.OpenFile(*keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("refused: %w", err)
	}
	privateKey := secp256k1.GenPrivKey()
	if _, err := handle.WriteString(hex.EncodeToString(privateKey.Key)); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return printPublic(privateKey)
}

func runPublic(arguments []string) error {
	flags := flag.NewFlagSet("public", flag.ContinueOnError)
	keyFile := flags.String("key-file", "", "key B file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	privateKey, err := readPrivateKeyFile(*keyFile)
	if err != nil {
		return err
	}
	return printPublic(privateKey)
}

func runSignDoc(arguments []string) error {
	flags := flag.NewFlagSet("sign-doc", flag.ContinueOnError)
	unsignedTxFile := flags.String("unsigned-tx", "", "output of --generate-only")
	chainId := flags.String("chain-id", "", "chain id")
	adminAddress := flags.String("admin", "", "network admin multisig address")
	accountNumber := flags.Uint64("account-number", 0, "admin account number")
	sequence := flags.Uint64("sequence", 0, "admin account sequence")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	txConfig, err := newTxConfig()
	if err != nil {
		return err
	}
	unsignedTxJson, err := os.ReadFile(*unsignedTxFile)
	if err != nil {
		return err
	}
	transaction, err := txConfig.TxJSONDecoder()(unsignedTxJson)
	if err != nil {
		return err
	}
	signable, ok := transaction.(authsigning.Tx)
	if !ok {
		return errors.New("unsigned tx is not signable")
	}
	signBytes, err := aminoJsonSignBytes(txConfig, signable, *chainId, *adminAddress, *accountNumber, *sequence)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"sign_doc_base64": base64.StdEncoding.EncodeToString(signBytes)})
}

func runServe(arguments []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configFile := flags.String("config", "", "daemon config JSON")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	configBytes, err := os.ReadFile(*configFile)
	if err != nil {
		return err
	}
	var config daemonConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return err
	}
	if _, err := sdk.AccAddressFromBech32(config.AdminAddress); err != nil || config.ChainId == "" {
		return errors.New("config needs chain_id and a bech32 admin_address")
	}
	host, _, err := net.SplitHostPort(config.Listen)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("listen must be a loopback address and port")
	}
	privateKey, err := readPrivateKeyFile(config.KeyFile)
	if err != nil {
		return err
	}
	txConfig, err := newTxConfig()
	if err != nil {
		return err
	}
	log.Printf("hub_admin_key_b listening on %s for %s, admin %s, key B %s", config.Listen, config.ChainId, config.AdminAddress, sdk.AccAddress(privateKey.PubKey().Address()).String())
	handler := http.NewServeMux()
	handler.HandleFunc("/sign", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, `{"refused": "POST only"}`, http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, maximumRequestBytes))
		if err != nil {
			http.Error(writer, `{"refused": "body unreadable"}`, http.StatusBadRequest)
			return
		}
		descriptorJson, typeUrls, requestId, err := signOneRequest(txConfig, privateKey, config, body)
		if err != nil {
			log.Printf("request %q refused: %v", requestId, err)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(writer).Encode(map[string]string{"request_id": requestId, "refused": err.Error()})
			return
		}
		log.Printf("request %q signed: %v", requestId, typeUrls)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(descriptorJson)
	})
	return http.ListenAndServe(config.Listen, handler)
}

func signOneRequest(txConfig client.TxConfig, privateKey *secp256k1.PrivKey, config daemonConfig, body []byte) ([]byte, []string, string, error) {
	var request signRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, nil, "", refusal("request is not JSON with request_id, unsigned_tx, account_number, sequence: %v", err)
	}
	transaction, typeUrls, err := checkedRoutineTx(txConfig, request.UnsignedTx, config.AdminAddress)
	if err != nil {
		return nil, nil, request.RequestId, err
	}
	signBytes, err := aminoJsonSignBytes(txConfig, transaction, config.ChainId, config.AdminAddress, request.AccountNumber, request.Sequence)
	if err != nil {
		return nil, nil, request.RequestId, err
	}
	descriptorJson, err := signatureDescriptorJson(txConfig, privateKey, signBytes, request.Sequence)
	return descriptorJson, typeUrls, request.RequestId, err
}
