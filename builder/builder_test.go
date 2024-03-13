package builder_test

import (
	"fmt"
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	testapp "github.com/polymerdao/monomer/testutil/app"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	tests := map[string]struct {
		inclusionList map[string]string
		mempool       map[string]string
	}{
		"no txs": {},
		"txs in inclusion list": {
			inclusionList: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		"txs in mempool": {
			mempool: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		"txs in mempool and inclusion list": {
			inclusionList: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
			mempool: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			formatTxs := func(diff map[string]string) bfttypes.Txs {
				var txs bfttypes.Txs
				for k, v := range diff {
					// TODO we should have helpers for this sort of thing.
					// `key=value` is how the test app expects transactions to be formatted.
					txs = append(txs, []byte(fmt.Sprintf("%s=%s", k, v)))	
				}
				return txs
			}
			inclusionListTxs := formatTxs(test.inclusionList)
			mempoolTxs := formatTxs(test.mempool)

			mempooldb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, mempooldb.Close())
			})
			pool := mempool.New(mempooldb)
			for _, tx := range mempoolTxs {
				require.NoError(t, pool.Enqueue(tx))
			}

			blockdb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, blockdb.Close())
			})
			blockStore := store.NewBlockStore(blockdb)

			txdb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, txdb.Close())
			})
			txStore := txstore.NewTxStore(txdb)

			app, err := testapp.NewApplication(testapp.DefaultConfig(t.TempDir()))
			require.NoError(t, err)

			eventBus := bfttypes.NewEventBus()
			require.NoError(t, eventBus.Start())
			t.Cleanup(func() {
				require.NoError(t, eventBus.Stop())
			})

			g := &genesis.Genesis{
				InitialL2Height: 1,
			}
			require.NoError(t, g.Commit(app, blockStore))

			b := builder.New(
				pool,
				app,
				blockStore,
				txStore,
				eventBus,
				g.ChainID,
			)

			require.NoError(t, b.Build(&builder.Payload{
				Transactions: inclusionListTxs,
				GasLimit:     0,
				Timestamp:    g.Time + 1,
			}))

			// Application state.

			// block store

			// tx store

			// event bus
		})
	}
}
