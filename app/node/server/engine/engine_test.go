package engine_test

/*
import (
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/polymerdao/monomer/app/peptide/store"
	ethengine "github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/app/node/server/engine"
	"github.com/stretchr/testify/require"
)
*/

type mockNode struct{}

func newMockNode() *mockNode {
	return &mockNode{}
}

/*
func TestForkchoiceUpdatedV3(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	payloadStore := payloadstore.NewPayloadStore()
	api := engine.NewEngineAPI(newMockNode(), blockStore)

	result, err := api.ForkchoiceUpdatedV3(eth.ForkchoiceState{
		HeadBlockHash: common.Hash{},
		SafeBlockHash: common.Hash{},
		FinalizedBlockHash: common.Hash{},
	}, eth.PayloadAttributes{})
	require.ErrorContains(t, err, ethengine.InvalidForkChoiceState.Error())
	require.Nil(t, result)
}
*/
