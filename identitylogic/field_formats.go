package identitylogic

import (
	"errors"
	"fmt"
	"regexp"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	lowercaseHexKeyLength   = 64
	lowercaseHexHashLength  = 64
	ownerSignatureHexLength = 130
	maximumEndpointLength   = 256
	maximumReasonLength     = 512
	maximumKindsPerCharter  = 32
	maximumOperators        = 32
)

const (
	TargetKindCharter    = "charter"
	TargetKindProvider   = "provider"
	TargetKindAnchor     = "anchor"
	ReasonCodeLegal      = "legal"
	ReasonCodeRegulatory = "regulatory"
	JoinOpen             = "open"
	JoinEndorsed         = "endorsed"
)

var (
	identifierFormat      = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	compactNameFormat     = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	lowercaseHexFormat    = regexp.MustCompile(`^[0-9a-f]+$`)
	ethereumAddressFormat = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
	siweNonceFormat       = regexp.MustCompile(`^[A-Za-z0-9]{8,64}$`)
	printableTextFormat   = regexp.MustCompile(`^[\x20-\x7e]*$`)
)

func ValidateIdentifier(fieldName string, value string) error {
	if !identifierFormat.MatchString(value) {
		return fmt.Errorf("%s must be 1 to 128 of letters, digits, dot, underscore, colon, dash", fieldName)
	}
	return nil
}

func ValidateCompactName(fieldName string, value string) error {
	if !compactNameFormat.MatchString(value) {
		return fmt.Errorf("%s must be 1 to 64 of letters, digits, dot, underscore, dash (no colon)", fieldName)
	}
	return nil
}

func validateLowercaseHexOfLength(fieldName string, value string, requiredLength int) error {
	if len(value) != requiredLength || !lowercaseHexFormat.MatchString(value) {
		return fmt.Errorf("%s must be %d lowercase hex characters without 0x", fieldName, requiredLength)
	}
	return nil
}

func ValidatePublicKeyHex(fieldName string, value string) error {
	return validateLowercaseHexOfLength(fieldName, value, lowercaseHexKeyLength)
}

func ValidateHashHex(fieldName string, value string) error {
	return validateLowercaseHexOfLength(fieldName, value, lowercaseHexHashLength)
}

func ValidateEthereumAddress(fieldName string, value string) error {
	if !ethereumAddressFormat.MatchString(value) {
		return fmt.Errorf("%s must be a 0x lowercase Ethereum address", fieldName)
	}
	return nil
}

func ValidateChainAddress(fieldName string, value string) error {
	if _, err := sdk.AccAddressFromBech32(value); err != nil {
		return fmt.Errorf("%s must be a chain address: %v", fieldName, err)
	}
	return nil
}

func ValidateEndpoint(endpoint string) error {
	if len(endpoint) > maximumEndpointLength || !printableTextFormat.MatchString(endpoint) {
		return fmt.Errorf("endpoint must be at most %d printable ASCII characters", maximumEndpointLength)
	}
	return nil
}

func ValidateKindList(fieldName string, kinds []string) error {
	if len(kinds) == 0 || len(kinds) > maximumKindsPerCharter {
		return fmt.Errorf("%s must list 1 to %d kinds", fieldName, maximumKindsPerCharter)
	}
	seenKinds := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		if err := ValidateCompactName(fieldName, kind); err != nil {
			return err
		}
		if seenKinds[kind] {
			return fmt.Errorf("%s lists %s twice", fieldName, kind)
		}
		seenKinds[kind] = true
	}
	return nil
}

func ValidateOperatorList(operators []string) error {
	if len(operators) > maximumOperators {
		return fmt.Errorf("at most %d operators", maximumOperators)
	}
	seenOperators := make(map[string]bool, len(operators))
	for _, operator := range operators {
		if err := ValidateChainAddress("operator", operator); err != nil {
			return err
		}
		if seenOperators[operator] {
			return fmt.Errorf("operator %s listed twice", operator)
		}
		seenOperators[operator] = true
	}
	return nil
}

func ValidateVetoReason(reasonCode string, reason string) error {
	if reasonCode != ReasonCodeLegal && reasonCode != ReasonCodeRegulatory {
		return errors.New("reason_code must be legal or regulatory")
	}
	if len(reason) == 0 || len(reason) > maximumReasonLength || !printableTextFormat.MatchString(reason) {
		return fmt.Errorf("reason must be 1 to %d printable ASCII characters", maximumReasonLength)
	}
	return nil
}

func ValidateJoinRule(join string) error {
	if join != JoinOpen && join != JoinEndorsed {
		return errors.New("join must be open or endorsed")
	}
	return nil
}

func Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
