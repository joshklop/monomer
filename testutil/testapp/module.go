package testapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v1 "github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
)

const Name = "module"

type module struct {
	key  *storetypes.KVStoreKey
	v1.UnimplementedMapServiceServer
}

func newModule(key *storetypes.KVStoreKey) *module {
	return &module{
		key: key,
	}
}

func (m *module) Init(ctx sdk.Context, data json.RawMessage) error {
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

func (m *module) Set(ctx context.Context, req *v1.SetRequest) (*v1.SetResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}

	sdkCtx.KVStore(m.key).Set([]byte(key), []byte(req.GetValue()))

	return &v1.SetResponse{}, nil
}
