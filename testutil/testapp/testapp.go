package testapp

import (
	"encoding/json"
	"fmt"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	testappv1 "github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
	"github.com/polymerdao/monomer/testutil/testapp/x/testmodule"
)

type App struct {
	*baseapp.BaseApp
}

func (a *App) RollbackToHeight(targetHeight uint64) error {
	return a.CommitMultiStore().RollbackToVersion(int64(targetHeight))
}

func New(appdb dbm.DB, chainID string, logger log.Logger) *App {
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*sdk.Msg)(nil), &testappv1.SetRequest{})
	ba := baseapp.NewBaseApp(
		"testapp",
		log.NewNopLogger(),
		appdb,
		tx.DefaultTxDecoder(codec.NewProtoCodec(interfaceRegistry)),
		baseapp.SetChainID(chainID),
	)

	key := sdk.NewKVStoreKey(testmodule.Name)
	module := testmodule.New(key)
	ba.MountKVStores(map[string]*storetypes.KVStoreKey{
		testmodule.Name: key,
	})

	ba.GRPCQueryRouter().SetInterfaceRegistry(interfaceRegistry)
	testappv1.RegisterGetServiceServer(ba.GRPCQueryRouter(), module)

	router := baseapp.NewMsgServiceRouter()
	router.SetInterfaceRegistry(interfaceRegistry)
	testappv1.RegisterSetServiceServer(router, module)
	ba.SetMsgServiceRouter(router)

	ba.SetInitChainer(func(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
		appStateBytes := req.GetAppStateBytes()
		if len(appStateBytes) == 0 {
			return abci.ResponseInitChain{}
		}

		genesis := make(map[string]json.RawMessage)
		if err := json.Unmarshal(appStateBytes, &genesis); err != nil {
			panic(fmt.Errorf("unmarshal genesis state: %v", err))
		}

		if moduleBytes, ok := genesis[testmodule.Name]; ok {
			if err := module.Init(ctx, moduleBytes); err != nil {
				panic(fmt.Errorf("initialize module: %v", err))
			}
		}

		return abci.ResponseInitChain{}
	})

	// If we don't LoadLatestVersion, the module store won't be loaded.
	// I don't understand the full meaning or implications of this.
	if err := ba.LoadLatestVersion(); err != nil {
		panic(fmt.Errorf("load latest version: %v", err))
	}

	return &App{
		BaseApp: ba,
	}
}
