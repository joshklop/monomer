package engine_test

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	dbm "github.com/cometbft/cometbft-db"
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

func TestChainId(t *testing.T) {
	db := dbm.NewMemDB()
	defer func() {
		require.NoError(t, db.Close())
	}()
	blockStore := store.NewBlockStore(db)
	engine.NewEthAPI(blockStore, newMockRegister(), "chainid", server.DefaultLogger())
}
