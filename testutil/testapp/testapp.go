package testapp

import (
	"encoding/json"
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	testappv1 "github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
	"github.com/stretchr/testify/require"
)

type App struct {
	*baseapp.BaseApp
}

func (a *App) RollbackToHeight(targetHeight uint64) error {
	return a.CommitMultiStore().RollbackToVersion(int64(targetHeight))
}

func New(t testing.TB, chainID string) *App {
	appdb := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, appdb.Close())
	})
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*sdk.Msg)(nil), &testappv1.SetRequest{})
	ba := baseapp.NewBaseApp(
		"testapp",
		log.NewNopLogger(),
		appdb,
		tx.DefaultTxDecoder(codec.NewProtoCodec(interfaceRegistry)),
		baseapp.SetChainID(chainID),
	)
	key := sdk.NewKVStoreKey(Name)
	module := newModule(key)
	ba.MountKVStores(map[string]*storetypes.KVStoreKey{
		Name: key,
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
		require.NoError(t, json.Unmarshal(appStateBytes, &genesis), "unmarshal genesis state")

		if moduleBytes, ok := genesis[Name]; ok {
			require.NoError(t, module.Init(ctx, moduleBytes), "initialize module")
		}

		return abci.ResponseInitChain{}
	})
	// If we don't LoadLatestVersion, the module store won't be loaded.
	// I don't understand the full meaning or implications of this.
	require.NoError(t, ba.LoadLatestVersion())
	return &App{
		BaseApp: ba,
	}
}
