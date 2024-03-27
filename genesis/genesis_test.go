package genesis_test

import (
	"encoding/json"
	"fmt"
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/testutil/testapp"
	"github.com/polymerdao/monomer/testutil/testapp/x/testmodule"
	"github.com/stretchr/testify/require"
)

func TestCommit(t *testing.T) {
	tests := map[string]*genesis.Genesis{
		"empty bytes state": {
			AppState: []byte{},
		},
		"empty json state": {
			AppState: []byte(`{}`),
		},
		"nonempty state": {
			AppState: []byte(fmt.Sprintf(`{"%s": { "test": "test" } }`, testmodule.Name)),
		},
		"non-zero chain ID": {
			ChainID: 1,
		},
		"non-zero genesis time": {
			Time: 1,
		},
	}

	for description, g := range tests {
		t.Run(description, func(t *testing.T) {
			app := testapp.NewTest(t, g.ChainID.String())

			blockstoredb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, blockstoredb.Close())
			})
			blockStore := store.NewBlockStore(blockstoredb)

			require.NoError(t, g.Commit(app, blockStore))

			// Application.
			info := app.Info(abci.RequestInfo{})
			require.Equal(t, int64(1), info.GetLastBlockHeight()) // This means that the genesis height was set correctly.
			state := make(map[string]map[string]string)           // It is ok to assume the format of the genesis state because it is defined by the test input.
			if len(g.AppState) > 0 {
				require.NoError(t, json.Unmarshal(g.AppState, &state))
			}
			gotState := make(map[string]map[string]string)
			for moduleName, moduleState := range state {
				gotState[moduleName] = make(map[string]string)
				app.StateContains(t, uint64(info.GetLastBlockHeight()), moduleState)
			}
			// Even though RequestInitChain contains the chain ID, we can't test that it was set properly since the ABCI doesn't expose it.

			// Block store.
			block := &eetypes.Block{
				Header: &eetypes.Header{
					ChainID:  g.ChainID,
					Height:   info.GetLastBlockHeight(),
					Time:     g.Time,
					AppHash:  info.GetLastBlockAppHash(),
					GasLimit: 30_000_000, // We cheat a little and copy the default gas limit here.
				},
			}
			block.Hash()
			require.Equal(t, block, blockStore.BlockByNumber(info.GetLastBlockHeight()))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Unsafe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Safe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Finalized))
		})
	}
}
