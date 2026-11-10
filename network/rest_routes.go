package network

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

type accountJsonBody struct {
	AccountId       string `json:"account_id"`
	RegistrationKey string `json:"registration_key"`
	OwnerAddress    string `json:"owner_address"`
	Expiry          uint64 `json:"expiry"`
}

type charterJsonBody struct {
	Subnet        string   `json:"subnet"`
	Version       uint64   `json:"version"`
	CharterHash   string   `json:"charter_hash"`
	ProviderKinds []string `json:"provider_kinds"`
	SegmentKinds  []string `json:"segment_kinds"`
	Operators     []string `json:"operators"`
	Join          string   `json:"join"`
	Height        uint64   `json:"height"`
	Vetoed        bool     `json:"vetoed"`
}

type providerJsonBody struct {
	Subnet          string `json:"subnet"`
	ProviderId      string `json:"provider_id"`
	ProviderKind    string `json:"provider_kind"`
	ProviderAddress string `json:"provider_address"`
	AccountId       string `json:"account_id"`
	Endpoint        string `json:"endpoint"`
	Height          uint64 `json:"height"`
	EndorsedHeight  uint64 `json:"endorsed_height"`
	Endorsed        bool   `json:"endorsed"`
	Vetoed          bool   `json:"vetoed"`
}

type endorsementJsonBody struct {
	Subnet     string `json:"subnet"`
	ProviderId string `json:"provider_id"`
	EndorserId string `json:"endorser_id"`
	Height     uint64 `json:"height"`
}

type vetoJsonBody struct {
	Subnet     string `json:"subnet"`
	TargetKind string `json:"target_kind"`
	TargetId   string `json:"target_id"`
	ReasonCode string `json:"reason_code"`
	Reason     string `json:"reason"`
	Height     uint64 `json:"height"`
}

type anchorJsonBody struct {
	Subnet      string `json:"subnet"`
	SegmentKind string `json:"segment_kind"`
	SegmentId   string `json:"segment_id"`
	ProviderId  string `json:"provider_id"`
	SegmentHash string `json:"segment_hash"`
	Height      uint64 `json:"height"`
	Endorsed    bool   `json:"endorsed"`
	Vetoed      bool   `json:"vetoed"`
}

func emptyListInsteadOfNull(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func charterBody(charter *Charter, vetoed bool) charterJsonBody {
	return charterJsonBody{Subnet: charter.Subnet, Version: charter.Version, CharterHash: charter.CharterHash,
		ProviderKinds: emptyListInsteadOfNull(charter.ProviderKinds), SegmentKinds: emptyListInsteadOfNull(charter.SegmentKinds),
		Operators: emptyListInsteadOfNull(charter.Operators), Join: charter.Join, Height: charter.Height, Vetoed: vetoed}
}

func providerBody(provider *Provider, vetoed bool) providerJsonBody {
	return providerJsonBody{Subnet: provider.Subnet, ProviderId: provider.ProviderId, ProviderKind: provider.ProviderKind,
		ProviderAddress: provider.ProviderAddress, AccountId: provider.AccountId, Endpoint: provider.Endpoint, Height: provider.Height,
		EndorsedHeight: provider.EndorsedHeight, Endorsed: provider.EndorsedHeight > 0, Vetoed: vetoed}
}

func vetoBody(veto *Veto) vetoJsonBody {
	return vetoJsonBody{Subnet: veto.Subnet, TargetKind: veto.TargetKind, TargetId: veto.TargetId, ReasonCode: veto.ReasonCode, Reason: veto.Reason, Height: veto.Height}
}

func anchorBody(anchor *Anchor) anchorJsonBody {
	return anchorJsonBody{Subnet: anchor.Subnet, SegmentKind: anchor.SegmentKind, SegmentId: anchor.SegmentId, ProviderId: anchor.ProviderId,
		SegmentHash: anchor.SegmentHash, Height: anchor.Height, Endorsed: anchor.Endorsed, Vetoed: anchor.Vetoed}
}

func FirstContactSigningBody(address sdk.AccAddress, existingAccount sdk.AccountI) map[string]any {
	if existingAccount != nil {
		return map[string]any{"address": address.String(), "exists": true,
			"account_number": strconv.FormatUint(existingAccount.GetAccountNumber(), 10), "sequence": existingAccount.GetSequence()}
	}
	return map[string]any{"address": address.String(), "exists": false,
		"account_number": strconv.FormatUint(FirstContactAccountNumber(address), 10), "sequence": uint64(0)}
}

func writeJsonResponse(writer http.ResponseWriter, statusCode int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(body)
}

func writeQueryError(writer http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	switch status.Code(err) {
	case codes.NotFound:
		statusCode = http.StatusNotFound
	case codes.InvalidArgument:
		statusCode = http.StatusBadRequest
	}
	writeJsonResponse(writer, statusCode, map[string]string{"error": err.Error()})
}

func RegisterRestRoutes(clientContext client.Context, router *mux.Router) {
	queryClient := NewQueryClient(clientContext)
	handle := func(path string, answer func(request *http.Request, pathValues map[string]string) (any, error)) {
		router.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
			body, err := answer(request, mux.Vars(request))
			if err != nil {
				writeQueryError(writer, err)
				return
			}
			writeJsonResponse(writer, http.StatusOK, body)
		}).Methods(http.MethodGet)
	}

	handle("/network/v1/account/{account_id}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Account(request.Context(), &QueryAccountRequest{AccountId: pathValues["account_id"]})
		if err != nil {
			return nil, err
		}
		account := response.Account
		return accountJsonBody{AccountId: account.AccountId, RegistrationKey: account.RegistrationKey, OwnerAddress: account.OwnerAddress, Expiry: account.Expiry}, nil
	})

	charterAnswer := func(request *http.Request, pathValues map[string]string) (any, error) {
		version := uint64(0)
		if versionText, given := pathValues["version"]; given {
			parsed, err := strconv.ParseUint(versionText, 10, 64)
			if err != nil || parsed == 0 {
				return nil, status.Error(codes.InvalidArgument, "version must be a positive whole number")
			}
			version = parsed
		}
		response, err := queryClient.Charter(request.Context(), &QueryCharterRequest{Subnet: pathValues["subnet"], Version: version})
		if err != nil {
			return nil, err
		}
		return charterBody(response.Charter, response.Vetoed), nil
	}
	handle("/network/v1/charter/{subnet}", charterAnswer)
	handle("/network/v1/charter/{subnet}/{version}", charterAnswer)

	handle("/network/v1/provider/{subnet}/{provider_id}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Provider(request.Context(), &QueryProviderRequest{Subnet: pathValues["subnet"], ProviderId: pathValues["provider_id"]})
		if err != nil {
			return nil, err
		}
		return providerBody(response.Provider, response.Vetoed), nil
	})

	handle("/network/v1/providers/{subnet}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Providers(request.Context(), &QueryProvidersRequest{Subnet: pathValues["subnet"], ProviderKind: request.URL.Query().Get("provider_kind")})
		if err != nil {
			return nil, err
		}
		bodies := []providerJsonBody{}
		for _, provider := range response.Providers {
			vetoResponse, err := queryClient.Veto(request.Context(), &QueryVetoRequest{Subnet: provider.Subnet, TargetKind: "provider", TargetId: provider.ProviderId})
			if err != nil && status.Code(err) != codes.NotFound {
				return nil, err
			}
			bodies = append(bodies, providerBody(provider, vetoResponse != nil && vetoResponse.Veto != nil))
		}
		return bodies, nil
	})

	handle("/network/v1/veto/{subnet}/{target_kind}/{target_id}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Veto(request.Context(), &QueryVetoRequest{Subnet: pathValues["subnet"], TargetKind: pathValues["target_kind"], TargetId: pathValues["target_id"]})
		if err != nil {
			return nil, err
		}
		return vetoBody(response.Veto), nil
	})

	handle("/network/v1/vetoes/{subnet}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Vetoes(request.Context(), &QueryVetoesRequest{Subnet: pathValues["subnet"]})
		if err != nil {
			return nil, err
		}
		bodies := []vetoJsonBody{}
		for _, veto := range response.Vetoes {
			bodies = append(bodies, vetoBody(veto))
		}
		return bodies, nil
	})

	handle("/network/v1/anchors/{subnet}/{segment_kind}/{segment_id}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Anchors(request.Context(), &QueryAnchorsRequest{Subnet: pathValues["subnet"], SegmentKind: pathValues["segment_kind"], SegmentId: pathValues["segment_id"]})
		if err != nil {
			return nil, err
		}
		bodies := []anchorJsonBody{}
		for _, anchor := range response.Anchors {
			bodies = append(bodies, anchorBody(anchor))
		}
		return bodies, nil
	})

	handle("/network/v1/anchor/{subnet}/{segment_kind}/{segment_id}/{provider_id}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Anchor(request.Context(), &QueryAnchorRequest{Subnet: pathValues["subnet"], SegmentKind: pathValues["segment_kind"], SegmentId: pathValues["segment_id"], ProviderId: pathValues["provider_id"]})
		if err != nil {
			return nil, err
		}
		return anchorBody(response.Anchor), nil
	})

	handle("/network/v1/endorsements/{subnet}/{provider_id}", func(request *http.Request, pathValues map[string]string) (any, error) {
		response, err := queryClient.Endorsements(request.Context(), &QueryEndorsementsRequest{Subnet: pathValues["subnet"], ProviderId: pathValues["provider_id"]})
		if err != nil {
			return nil, err
		}
		bodies := []endorsementJsonBody{}
		for _, endorsement := range response.Endorsements {
			bodies = append(bodies, endorsementJsonBody{Subnet: endorsement.Subnet, ProviderId: endorsement.ProviderId, EndorserId: endorsement.EndorserId, Height: endorsement.Height})
		}
		return bodies, nil
	})

	authQueryClient := authtypes.NewQueryClient(clientContext)
	router.HandleFunc("/network/v1/first_contact/{address}", func(writer http.ResponseWriter, request *http.Request) {
		address, err := sdk.AccAddressFromBech32(mux.Vars(request)["address"])
		if err != nil {
			writeJsonResponse(writer, http.StatusBadRequest, map[string]string{"error": "address must be a chain address"})
			return
		}
		var existingAccount sdk.AccountI
		response, err := authQueryClient.Account(request.Context(), &authtypes.QueryAccountRequest{Address: address.String()})
		switch {
		case err == nil:
			if err := clientContext.InterfaceRegistry.UnpackAny(response.Account, &existingAccount); err != nil {
				writeQueryError(writer, status.Error(codes.Internal, err.Error()))
				return
			}
		case status.Code(err) != codes.NotFound:
			writeQueryError(writer, err)
			return
		}
		writeJsonResponse(writer, http.StatusOK, FirstContactSigningBody(address, existingAccount))
	}).Methods(http.MethodGet)
}
