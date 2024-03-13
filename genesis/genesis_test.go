package genesis_test

import (
	"encoding/json"
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/genesis"
	testapp "github.com/polymerdao/monomer/testutil/app"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	require.NoError(t, (&genesis.Genesis{
		InitialL2Height: 1,
	}).Validate())
	require.ErrorContains(t, (&genesis.Genesis{}).Validate(), "initial L2 height must be non-zero")
}

func TestCommit(t *testing.T) {
	tests := map[string]*genesis.Genesis{
		"empty non-nil state": {
			InitialL2Height: 1,
			AppState: []byte{},
		},
		"nonempty state": {
			InitialL2Height: 1,
			AppState: func() []byte {
				// We happen to know this is the proper way to marshal the testapp's state.
				// It is a pity the testapp doesn't have a better way to programmatically create states.
				stateBytes, err := json.Marshal(map[string]string{
					"test": "test",
				})
				require.NoError(t, err)
				return stateBytes
			}(),
		},
		"non-zero chain ID": {
			InitialL2Height: 1,
			ChainID: 1,
		},
		"non-zero genesis time": {
			InitialL2Height: 1,
			Time: 1,
		},
	}

	for description, g := range tests {
		t.Run(description, func(t *testing.T) {
			app, err := testapp.NewApplication(testapp.DefaultConfig(t.TempDir()))
			require.NoError(t, err)
			blockstoredb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, blockstoredb.Close())
			})
			blockStore := store.NewBlockStore(blockstoredb)

			require.NoError(t, g.Validate())
			require.NoError(t, g.Commit(app, blockStore))

			// Application.
			if len(g.AppState) > 0 {
				state := make(map[string]string)
				require.NoError(t, json.Unmarshal(g.AppState, &state))
				// State has been committed.
				for k, v := range state {
					resp := app.Query(abci.RequestQuery{
						Data: []byte(k),
					})
					require.Equal(t, v, string(resp.GetValue()))
				}
			}
			info := app.Info(abci.RequestInfo{})
			require.Equal(t, int64(1), info.GetLastBlockHeight()) // This means that the genesis height was set correctly.
			// Even though RequestInitChain contains the chain ID, we can't test that it was set properly since the ABCI doesn't expose it.

			// Block store.
			block := &eetypes.Block{
				Header: &eetypes.Header{
					ChainID:  g.ChainID,
					Height:   int64(g.InitialL2Height),
					Time:     g.Time,
					AppHash:  info.GetLastBlockAppHash(),
					GasLimit: peptide.DefaultGasLimit,
				},
			}
			block.Hash()
			require.Equal(t, block, blockStore.BlockByNumber(int64(g.InitialL2Height)))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Unsafe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Safe))
			require.Equal(t, block, blockStore.BlockByLabel(eth.Finalized))
		})
	}
}
