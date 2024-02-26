package engine_test

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	eetypes "github.com/polymerdao/monomer/app/node/types"
	dbm "github.com/cometbft/cometbft-db"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/app/node/server"
	"github.com/polymerdao/monomer/app/node/server/engine"
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
	api := engine.NewEthAPI(blockStore, newMockRegister(), &chainID, server.DefaultLogger())
	require.Equal(t, &chainID, api.ChainId())
}

func TestGetBalance(t *testing.T) {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	blockStore := store.NewBlockStore(db)
	register := newMockRegister()
	addr := common.HexToAddress("0x1")
	balance := big.NewInt(2)
	blockStore.AddBlock(&eetypes.Block{
		Header: &eetypes.Header{
			Height: 0,
			ChainID: chainID.ToInt().String(),
		},
		BlockHash: common.Hash{0},
	})
	headHash := common.Hash{1}
	blockStore.AddBlock(&eetypes.Block{
		Header: &eetypes.Header{
			Height: 1,
			ChainID: chainID.ToInt().String(),
		},
		BlockHash: headHash,
	})
	// TODO we should technically have to update the block label here to
	// make the above blocks part of the canonical chain. The fact that this test passes
	// anyway shows that this is a bug in the implementation.
	register.SetBalance(addr, 1, balance)

	api := engine.NewEthAPI(blockStore, register, &chainID, server.DefaultLogger())
	got, err := api.GetBalance(addr, 1)
	require.NoError(t, err)
	require.Equal(t, balance, got.ToInt())
}
