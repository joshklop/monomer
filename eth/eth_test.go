package eth_test

import (
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/eth"
	"github.com/stretchr/testify/require"
)

func TestChainId(t *testing.T) {
	for _, id := range []eetypes.ChainID{0, 1, 2, 10} {
		t.Run(id.String(), func(t *testing.T) {
			hexID := id.HexBig()
			require.Equal(t, hexID, eth.NewChainID(hexID).ChainId())
		})
	}
}

func TestGetBlockByNumber(t *testing.T) {
	db := dbm.NewMemDB()
	defer func() {
		require.NoError(t, db.Close())
	}()
	blockStore := store.NewBlockStore(db)
	block := &eetypes.Block{
		Header: &eetypes.Header{
			Hash: common.Hash{1},
		},
	}
	blockStore.AddBlock(block)
	require.NoError(t, blockStore.UpdateLabel(opeth.Unsafe, block.Header.Hash))

	tests := map[string]struct {
		id   eth.BlockID
		want *eetypes.Block
	}{
		"unsafe block exists": {
			id: eth.BlockID{
				Label: opeth.Unsafe,
			},
			want: block,
		},
		"safe block does not exist": {
			id: eth.BlockID{
				Label: opeth.Safe,
			},
		},
		"finalized block does not exist": {
			id: eth.BlockID{
				Label: opeth.Finalized,
			},
		},
		"block 0 exists": {
			id:   eth.BlockID{},
			want: block,
		},
		"block 1 does not exist": {
			id: eth.BlockID{
				Height: 1,
			},
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			for description, includeTxs := range map[string]bool{
				"include txs": true,
				"exclude txs": false,
			} {
				t.Run(description, func(t *testing.T) {
					s := eth.NewBlockByNumber(blockStore)
					got, err := s.GetBlockByNumber(test.id, includeTxs)
					if test.want == nil {
						require.Error(t, err)
						require.Nil(t, got)
						return
					}
					require.NoError(t, err)
					require.Equal(t, test.want.ToEthLikeBlock(includeTxs), got)
				})
			}
		})
	}
}

func TestGetBlockByHash(t *testing.T) {
	db := dbm.NewMemDB()
	defer func() {
		require.NoError(t, db.Close())
	}()
	blockStore := store.NewBlockStore(db)
	block := &eetypes.Block{
		Header: &eetypes.Header{
			Hash: common.Hash{1},
		},
	}
	blockStore.AddBlock(block)

	for description, inclTx := range map[string]bool{
		"include txs": true,
		"exclude txs": false,
	} {
		t.Run(description, func(t *testing.T) {
			t.Run("block hash 1 exists", func(t *testing.T) {
				e := eth.NewBlockByHash(blockStore)
				got, err := e.GetBlockByHash(block.Header.Hash, inclTx)
				require.NoError(t, err)
				require.Equal(t, block.ToEthLikeBlock(inclTx), got)
			})
		})
		t.Run("block hash 0 does not exist", func(t *testing.T) {
			for description, inclTx := range map[string]bool{
				"include txs": true,
				"exclude txs": false,
			} {
				t.Run(description, func(t *testing.T) {
					e := eth.NewBlockByHash(blockStore)
					got, err := e.GetBlockByHash(common.Hash{}, inclTx)
					require.Nil(t, got)
					require.ErrorContains(t, err, "not found")
				})
			}
		})
	}
}
