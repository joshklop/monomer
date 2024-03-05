package engine_test

import (
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/app/node/server/engine"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/stretchr/testify/require"
)

type mockNode struct{}

func newMockNode() *mockNode {
	return &mockNode{}
}

func (n *mockNode) LastBlockHeight() int64 {
	return 0
}

func (n *mockNode) HeadBlockHash() common.Hash {
	return common.Hash{}
}

func (n *mockNode) CommitBlock() error {
	return nil
}

func (n *mockNode) UpdateLabel(label eth.BlockLabel, hash common.Hash) error {
	return nil
}

func (n *mockNode) Rollback(head, safe, finalized *eetypes.Block) error {
	return nil
}

func TestForkchoiceUpdatedV3(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	payloadStore := payloadstore.NewPayloadStore()
	api := engine.NewEngineAPI(newMockNode(), blockStore, payloadStore)

	// TODO
	_, _ = api.ForkchoiceUpdatedV3(eth.ForkchoiceState{
		HeadBlockHash: common.Hash{},
		SafeBlockHash: common.Hash{},
		FinalizedBlockHash: common.Hash{},
	}, eth.PayloadAttributes{})
	//require.NoError(t, err)
}
