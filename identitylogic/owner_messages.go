package identitylogic

import (
	"errors"
	"time"
)

type OwnerClaim struct {
	ChainId         string
	AccountId       string
	RegistrationKey string
	OwnerAddress    string
	Nonce           string
	IssuedAt        uint64
	ExpiresAt       uint64
}

type ProviderRegistration struct {
	ChainId         string
	Subnet          string
	ProviderId      string
	ProviderKind    string
	ProviderAddress string
	Endpoint        string
}

func formatSiweTime(unixSeconds uint64) string {
	return time.Unix(int64(unixSeconds), 0).UTC().Format(time.RFC3339)
}

func OwnerClaimText(claim OwnerClaim) (string, error) {
	checksummedAddress, err := ChecksummedAddress(claim.OwnerAddress)
	if err != nil {
		return "", err
	}
	return "traderswift wants you to sign in with your Ethereum account:\n" +
		checksummedAddress + "\n\n" +
		"Claim traderswift account " + claim.AccountId + " with registration key " + claim.RegistrationKey + "\n\n" +
		"URI: traderswift://" + claim.ChainId + "/claim\n" +
		"Version: 1\n" +
		"Chain ID: 1\n" +
		"Nonce: " + claim.Nonce + "\n" +
		"Issued At: " + formatSiweTime(claim.IssuedAt) + "\n" +
		"Expiration Time: " + formatSiweTime(claim.ExpiresAt), nil
}

func ValidateOwnerClaimWindow(claim OwnerClaim, blockTimeSeconds uint64) error {
	if !siweNonceFormat.MatchString(claim.Nonce) {
		return errors.New("nonce must be 8 to 64 letters or digits")
	}
	if claim.ExpiresAt <= claim.IssuedAt {
		return errors.New("expires_at must be after issued_at")
	}
	if blockTimeSeconds >= claim.ExpiresAt {
		return errors.New("claim message has expired")
	}
	return nil
}

func ProviderRegistrationText(registration ProviderRegistration) string {
	return "traderswift register provider\n" +
		"chain id: " + registration.ChainId + "\n" +
		"subnet: " + registration.Subnet + "\n" +
		"provider id: " + registration.ProviderId + "\n" +
		"provider kind: " + registration.ProviderKind + "\n" +
		"provider address: " + registration.ProviderAddress + "\n" +
		"endpoint: " + registration.Endpoint
}
