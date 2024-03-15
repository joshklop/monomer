package builder_test

import (
	"context"
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmtpubsub "github.com/cometbft/cometbft/libs/pubsub"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	testapp "github.com/polymerdao/monomer/testutil/app"
	"github.com/stretchr/testify/require"
)

type queryAll struct{}

var _ cmtpubsub.Query = (*queryAll)(nil)

func (*queryAll) Matches(_ map[string][]string) (bool, error) {
	return true, nil
}

func (*queryAll) String() string {
	return "all"
}

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
			inclusionListTxs := testapp.ToTxs(test.inclusionList)
			mempoolTxs := testapp.ToTxs(test.mempool)

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
			subscription, err := eventBus.Subscribe(context.Background(), "test", &queryAll{}, len(test.mempool)+len(test.inclusionList)+1)
			require.NoError(t, err)

			g := &genesis.Genesis{}
			require.NoError(t, g.Commit(app, blockStore))
			appHash := app.Info(abcitypes.RequestInfo{}).LastBlockAppHash

			b := builder.New(
				pool,
				app,
				blockStore,
				txStore,
				eventBus,
				g.ChainID,
			)

			payload := &builder.Payload{
				Transactions: bfttypes.ToTxs(inclusionListTxs),
				GasLimit:     0,
				Timestamp:    g.Time + 1,
			}
			require.NoError(t, b.Build(payload))

			// Application.
			require.NoError(t, testapp.ValuesMatch(app, test.inclusionList), "all txs in inclusion list should be executed")
			require.NoError(t, testapp.ValuesMatch(app, test.mempool), "all txs in mempool should be executed")

			// Block store.
			genesisBlock := blockStore.BlockByNumber(0)
			gotBlock := blockStore.HeadBlock()
			wantBlock := &eetypes.Block{
				Header: &eetypes.Header{
					ChainID:    g.ChainID,
					Height:     1,
					Time:       payload.Timestamp,
					ParentHash: genesisBlock.Hash(),
					AppHash:    appHash,
					GasLimit:   payload.GasLimit,
				},
				Txs: bfttypes.ToTxs(append(inclusionListTxs, mempoolTxs...)),
			}
			wantBlock.Hash()
			require.Equal(t, wantBlock, gotBlock)

			// Tx store.
			var txResults []*abcitypes.TxResult
			for i, tx := range wantBlock.Txs {
				got, err := txStore.Get(tx.Hash())
				require.NoError(t, err)
				result := &abcitypes.TxResult{
					Height: wantBlock.Header.Height,
					Index:  uint32(i),
					Tx:     tx,
					Result: testapp.SuccessfulDeliverTx,
				}
				require.Equal(t, result, got)
				txResults = append(txResults, result)
			}

			// Event bus.
			eventChan := subscription.Out()
			require.Len(t, eventChan, len(txResults))
			for _, result := range txResults {
				event := <-eventChan
				data := event.Data()
				require.IsType(t, bfttypes.EventDataTx{}, data)
				require.Equal(t, bfttypes.EventDataTx{
					TxResult: *result,
				}, data.(bfttypes.EventDataTx))
			}
			require.NoError(t, subscription.Err())
		})
	}
}

func TestRollback(t *testing.T) {
	// 1:
	//  - unsafe, safe, finalized updated to block 2
	//  - rollback all labels to block 1
	// 2:
	//  - unsafe, safe to block 2
	//  - rollback unsafe and safe to block 1
	// 3:
	//  - unsafe to block 2
	//  - rollback unsafe to block 1

	tests := map[string]struct {
		update   []eth.BlockLabel
		rollback []eth.BlockLabel
	}{
		"update all, rollback all": {
			update:   []eth.BlockLabel{eth.Unsafe, eth.Safe, eth.Finalized},
			rollback: []eth.BlockLabel{eth.Unsafe, eth.Safe, eth.Finalized},
		},
		"update unsafe and safe, rollback unsafe and safe": {
			update:   []eth.BlockLabel{eth.Unsafe, eth.Safe},
			rollback: []eth.BlockLabel{eth.Unsafe, eth.Safe},
		},
		"update unsafe, rollback unsafe": {
			update:   []eth.BlockLabel{eth.Unsafe},
			rollback: []eth.BlockLabel{eth.Unsafe},
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			mempooldb := tmdb.NewMemDB()
			t.Cleanup(func() {
				require.NoError(t, mempooldb.Close())
			})
			pool := mempool.New(mempooldb)

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

			g := &genesis.Genesis{}
			require.NoError(t, g.Commit(app, blockStore))

			b := builder.New(
				pool,
				app,
				blockStore,
				txStore,
				eventBus,
				g.ChainID,
			)

			kvs := map[string]string{
				"test": "test",
			}
			require.NoError(t, b.Build(&builder.Payload{
				Timestamp:    g.Time + 1,
				Transactions: bfttypes.ToTxs(testapp.ToTxs(kvs)),
			}))
			block := blockStore.HeadBlock()
			require.NotNil(t, block)
			for _, label := range test.update {
				require.NoError(t, blockStore.UpdateLabel(label, block.Hash()))
			}
			genesisBlock := blockStore.BlockByNumber(0)

			{
				hashes := map[eth.BlockLabel]common.Hash{
					eth.Unsafe:    block.Hash(),
				}
				for _, label := range test.rollback {
					hashes[label] = genesisBlock.Hash()
				}
				require.NoError(t, b.Rollback(hashes[eth.Unsafe], hashes[eth.Safe], hashes[eth.Finalized]))
			}

			// Application.
			for k := range kvs {
				resp := app.Query(abcitypes.RequestQuery{
					Data: []byte(k),
				})
				require.Empty(t, resp.GetValue()) // Value was removed from state.
			}

			// Block store.
			headBlock := blockStore.HeadBlock()
			require.NotNil(t, headBlock)
			require.Equal(t, 0, uint64(headBlock.Header.Height))
			// We trust that the other parts of a block store rollback were done as well.

			// Tx store.
			for _, tx := range bfttypes.ToTxs(testapp.ToTxs(kvs)) {
				result, err := txStore.Get(tx.Hash())
				require.NoError(t, err)
				require.Nil(t, result)
			}
			// We trust that the other parts of a tx store rollback were done as well.
		})
	}
}
