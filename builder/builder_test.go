package builder_test

import (
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/store"
	testapp "github.com/polymerdao/monomer/testutil/app"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/mempool"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
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

	testApp, err := testapp.NewApplication(testapp.DefaultConfig(t.TempDir()))
	require.NoError(t, err)
	
	eventBus := bfttypes.NewEventBus()
	require.NoError(t, eventBus.Start())
	t.Cleanup(func() {
		require.NoError(t, eventBus.Stop())
	})

	// TODO need to set up genesis.

	builder := builder.New(
		pool,
		testApp,
		blockStore,
		txStore,
		eventBus,
		"1",
	)
}
