package eth_test

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer/app/node/server/engine"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/stretchr/testify/require"
)

type mockRegister struct {
	mu       sync.Mutex
	register map[common.Address]map[int64]*big.Int
}

func newMockRegister() *mockRegister {
	return &mockRegister{
		register: make(map[common.Address]map[int64]*big.Int),
	}
}

func (mr *mockRegister) SetBalance(addr common.Address, height int64, val *big.Int) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	heightMapping, ok := mr.register[addr]
	if !ok {
		heightMapping = make(map[int64]*big.Int)
	}
	heightMapping[height] = val
	mr.register[addr] = heightMapping
}

func (mr *mockRegister) Balance(addr common.Address, height int64) (*big.Int, error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	heightMapping, ok := mr.register[addr]
	if !ok {
		return nil, fmt.Errorf("no balance found at address %s", addr)
	}
	return heightMapping[height], nil
}

var chainID = hexutil.Big(*big.NewInt(1))

func TestChainId(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	api := engine.NewEthAPI(blockStore, newMockRegister(), &chainID)
	require.Equal(t, &chainID, api.ChainId())
}

// TODO we should test all block ids
func TestGetBalance(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	blockStore.AddBlock(&eetypes.Block{
		Header: &eetypes.Header{
			Height:  0,
			ChainID: eetypes.ChainID(chainID.ToInt().Uint64()),
			Hash:    common.Hash{0},
		},
	})
	blockStore.AddBlock(&eetypes.Block{
		Header: &eetypes.Header{
			Height:  1,
			ChainID: eetypes.ChainID(chainID.ToInt().Uint64()),
			Hash:    common.Hash{1},
		},
	})
	// TODO we should technically have to update the block label here to
	// make the above blocks part of the canonical chain. The fact that this test passes
	// anyway shows that this is a bug in the implementation.
	register := newMockRegister()
	addr := common.HexToAddress("0x1")
	balance0 := big.NewInt(2)
	register.SetBalance(addr, 0, balance0)
	balance1 := new(big.Int).Add(balance0, big.NewInt(1))
	register.SetBalance(addr, 1, balance1)
	api := engine.NewEthAPI(blockStore, register, &chainID)

	got0, err := api.GetBalance(addr, 0)
	require.NoError(t, err)
	require.Equal(t, balance0, got0.ToInt())

	got1, err := api.GetBalance(addr, 1)
	require.NoError(t, err)
	require.Equal(t, balance1, got1.ToInt())
}

func TestGetBlockByHash(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	b := &eetypes.Block{
		Header: &eetypes.Header{
			Height:     1,
			ChainID:    eetypes.ChainID(chainID.ToInt().Uint64()),
			AppHash:    []byte{1},
			Time:       4,
			ParentHash: common.Hash{2},
			Hash:       common.Hash{1},
		},
		Txs: bfttypes.Txs{bfttypes.Tx([]byte{1})},
	}
	blockStore.AddBlock(b)
	register := newMockRegister()
	api := engine.NewEthAPI(blockStore, register, &chainID)

	got, err := api.GetBlockByHash(b.Header.Hash, false)
	require.NoError(t, err)
	require.Equal(t, b.ToEthLikeBlock(false), got)

	got, err = api.GetBlockByHash(b.Header.Hash, true)
	require.NoError(t, err)
	require.Equal(t, got["transactions"].(ethtypes.Transactions).Len(), b.Txs.Len())
	delete(got, "transactions") // Deep equality won't work since the internal tx timestamps are different.
	want := b.ToEthLikeBlock(true)
	delete(want, "transactions")
	require.Equal(t, want, got)
}

func TestGetBlockByNumber(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	b := &eetypes.Block{
		Header: &eetypes.Header{
			Height:     1,
			ChainID:    eetypes.ChainID(chainID.ToInt().Uint64()),
			AppHash:    []byte{1},
			Time:       4,
			ParentHash: common.Hash{2},
			Hash:       common.Hash{1},
		},
		Txs: bfttypes.Txs{bfttypes.Tx([]byte{1})},
	}
	blockStore.AddBlock(b)
	register := newMockRegister()
	api := engine.NewEthAPI(blockStore, register, &chainID)

	got, err := api.GetBlockByNumber(b.Header.Height, false)
	require.NoError(t, err)
	require.Equal(t, b.ToEthLikeBlock(false), got)

	got, err = api.GetBlockByNumber(b.Header.Height, true)
	require.NoError(t, err)
	require.Equal(t, got["transactions"].(ethtypes.Transactions).Len(), b.Txs.Len())
	delete(got, "transactions") // Deep equality won't work since the internal tx timestamps are different.
	want := b.ToEthLikeBlock(true)
	delete(want, "transactions")
	require.Equal(t, want, got)
}

func TestGetBlockWithNilID(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	b := &eetypes.Block{
		Header: &eetypes.Header{
			Height:     1,
			ChainID:    eetypes.ChainID(chainID.ToInt().Uint64()),
			AppHash:    []byte{1},
			Time:       4,
			ParentHash: common.Hash{2},
			Hash:       common.Hash{1},
		},
		Txs: bfttypes.Txs{bfttypes.Tx([]byte{1})},
	}
	blockStore.AddBlock(b)
	require.NoError(t, blockStore.UpdateLabel(eth.Unsafe, b.Hash()))
	api := engine.NewEthAPI(blockStore, newMockRegister(), &chainID)

	want, err := api.GetBlockByNumber(eth.Unsafe, false)
	require.NoError(t, err)
	got, err := api.GetBlockByNumber(nil, false)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestGetBlockByLabel(t *testing.T) {
	for _, label := range []eth.BlockLabel{eth.Unsafe, eth.Safe, eth.Finalized} {
		db := dbm.NewMemDB()
		t.Cleanup(func() {
			require.NoError(t, db.Close())
		})
		blockStore := store.NewBlockStore(db)
		b := &eetypes.Block{
			Header: &eetypes.Header{
				Height:     1,
				ChainID:    eetypes.ChainID(chainID.ToInt().Uint64()),
				AppHash:    []byte{1},
				Time:       4,
				ParentHash: common.Hash{2},
				Hash:       common.Hash{1},
			},
			Txs: bfttypes.Txs{bfttypes.Tx([]byte{1})},
		}
		blockStore.AddBlock(b)
		require.NoError(t, blockStore.UpdateLabel(label, b.Hash()))
		register := newMockRegister()
		api := engine.NewEthAPI(blockStore, register, &chainID)

		got, err := api.GetBlockByNumber(label, false)
		require.NoError(t, err)
		require.Equal(t, b.ToEthLikeBlock(false), got)

		got, err = api.GetBlockByNumber(label, true)
		require.NoError(t, err)
		require.Equal(t, got["transactions"].(ethtypes.Transactions).Len(), b.Txs.Len())
		delete(got, "transactions") // Deep equality won't work since the internal tx timestamps are different.
		want := b.ToEthLikeBlock(true)
		delete(want, "transactions")
		require.Equal(t, want, got)
	}
}