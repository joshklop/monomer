package testmodule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
)

const Name = "module"

type Module struct {
	key storetypes.StoreKey
	testappv1.UnimplementedSetServiceServer
	testappv1.UnimplementedGetServiceServer
}

func New(key *storetypes.KVStoreKey) *Module {
	return &Module{
		key: key,
	}
}

func (m *Module) Init(ctx sdk.Context, data json.RawMessage) error {
	if data == nil {
		return nil
	}
	genesis := make(map[string]string)
	if err := json.Unmarshal(data, &genesis); err != nil {
		return fmt.Errorf("unmarshal genesis data: %v", err)
	}
	store := ctx.KVStore(m.key)
	for k, v := range genesis {
		store.Set([]byte(k), []byte(v))
	}
	return nil
}

func (m *Module) Get(ctx context.Context, req *testappv1.GetRequest) (*testappv1.GetResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}
	return &testappv1.GetResponse{
		Value: string(sdkCtx.KVStore(m.key).Get([]byte(key))),
	}, nil
}

func (m *Module) Set(ctx context.Context, req *testappv1.SetRequest) (*testappv1.SetResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}
	sdkCtx.KVStore(m.key).Set([]byte(key), []byte(req.GetValue()))
	return &testappv1.SetResponse{}, nil
}
