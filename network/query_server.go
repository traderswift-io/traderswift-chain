package network

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"

	"traderswift-chain/identitylogic"
)

type queryServer struct {
	keeper *Keeper
}

func NewQueryServer(keeper *Keeper) QueryServer {
	return &queryServer{keeper: keeper}
}

func notFoundOrInternal(err error, what string) error {
	if errors.Is(err, collections.ErrNotFound) {
		return status.Errorf(codes.NotFound, "%s not found", what)
	}
	return status.Error(codes.Internal, err.Error())
}

func (server *queryServer) Account(ctx context.Context, request *QueryAccountRequest) (*QueryAccountResponse, error) {
	account, err := server.keeper.Accounts.Get(ctx, request.AccountId)
	if err != nil {
		return nil, notFoundOrInternal(err, "account")
	}
	return &QueryAccountResponse{Account: &account}, nil
}

func (server *queryServer) Charter(ctx context.Context, request *QueryCharterRequest) (*QueryCharterResponse, error) {
	version := request.Version
	if version == 0 {
		currentVersion, err := server.keeper.CurrentCharterVersion.Get(ctx, request.Subnet)
		if err != nil {
			return nil, notFoundOrInternal(err, "charter")
		}
		version = currentVersion
	}
	charter, err := server.keeper.Charters.Get(ctx, collections.Join(request.Subnet, version))
	if err != nil {
		return nil, notFoundOrInternal(err, "charter")
	}
	vetoed, err := server.keeper.isVetoed(ctx, request.Subnet, identitylogic.TargetKindCharter, CharterVetoTargetId(version))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &QueryCharterResponse{Charter: &charter, Vetoed: vetoed}, nil
}

func (server *queryServer) Provider(ctx context.Context, request *QueryProviderRequest) (*QueryProviderResponse, error) {
	provider, err := server.keeper.Providers.Get(ctx, collections.Join(request.Subnet, request.ProviderId))
	if err != nil {
		return nil, notFoundOrInternal(err, "provider")
	}
	vetoed, err := server.keeper.isVetoed(ctx, request.Subnet, identitylogic.TargetKindProvider, request.ProviderId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &QueryProviderResponse{Provider: &provider, Vetoed: vetoed}, nil
}

func (server *queryServer) Providers(ctx context.Context, request *QueryProvidersRequest) (*QueryProvidersResponse, error) {
	response := &QueryProvidersResponse{Providers: []*Provider{}}
	err := server.keeper.Providers.Walk(ctx, collections.NewPrefixedPairRange[string, string](request.Subnet), func(_ collections.Pair[string, string], provider Provider) (bool, error) {
		if request.ProviderKind == "" || provider.ProviderKind == request.ProviderKind {
			response.Providers = append(response.Providers, &provider)
		}
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return response, nil
}

func (server *queryServer) Veto(ctx context.Context, request *QueryVetoRequest) (*QueryVetoResponse, error) {
	veto, err := server.keeper.Vetoes.Get(ctx, collections.Join3(request.Subnet, request.TargetKind, request.TargetId))
	if err != nil {
		return nil, notFoundOrInternal(err, "veto")
	}
	return &QueryVetoResponse{Veto: &veto}, nil
}

func (server *queryServer) Vetoes(ctx context.Context, request *QueryVetoesRequest) (*QueryVetoesResponse, error) {
	response := &QueryVetoesResponse{Vetoes: []*Veto{}}
	err := server.keeper.Vetoes.Walk(ctx, collections.NewPrefixedTripleRange[string, string, string](request.Subnet), func(_ collections.Triple[string, string, string], veto Veto) (bool, error) {
		response.Vetoes = append(response.Vetoes, &veto)
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return response, nil
}

func (server *queryServer) Anchor(ctx context.Context, request *QueryAnchorRequest) (*QueryAnchorResponse, error) {
	anchor, err := server.keeper.Anchors.Get(ctx, anchorKeyOf(request.Subnet, request.SegmentKind, request.SegmentId, request.ProviderId))
	if err != nil {
		return nil, notFoundOrInternal(err, "anchor")
	}
	return &QueryAnchorResponse{Anchor: &anchor}, nil
}

func (server *queryServer) Anchors(ctx context.Context, request *QueryAnchorsRequest) (*QueryAnchorsResponse, error) {
	response := &QueryAnchorsResponse{Anchors: []*Anchor{}}
	segmentRange := collections.NewSuperPrefixedQuadRange3[string, string, string, string](request.Subnet, request.SegmentKind, request.SegmentId)
	err := server.keeper.Anchors.Walk(ctx, segmentRange, func(_ collections.Quad[string, string, string, string], anchor Anchor) (bool, error) {
		response.Anchors = append(response.Anchors, &anchor)
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return response, nil
}

func (server *queryServer) Endorsements(ctx context.Context, request *QueryEndorsementsRequest) (*QueryEndorsementsResponse, error) {
	response := &QueryEndorsementsResponse{Endorsements: []*Endorsement{}}
	providerRange := collections.NewSuperPrefixedTripleRange[string, string, string](request.Subnet, request.ProviderId)
	err := server.keeper.Endorsements.Walk(ctx, providerRange, func(_ collections.Triple[string, string, string], endorsement Endorsement) (bool, error) {
		response.Endorsements = append(response.Endorsements, &endorsement)
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return response, nil
}
