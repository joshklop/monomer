package engine_test

import (
	"sync"
	"testing"

	"github.com/polymerdao/monomer/app/node/server/engine"
	"github.com/polymerdao/monomer/app/node/server"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymerdao/monomer/app/peptide/store"
	dbm "github.com/cometbft/cometbft-db"
	"github.com/stretchr/testify/require"
)

type mockRegister struct {
	mu sync.Mutex
	register map[common.Address]map[int64]uint64
}

func newMockRegister() *mockRegister {
	return &mockRegister{
		register: make(map[common.Address]map[int64]uint64),
	}
}

func (mr *mockRegister) SetBalance(addr common.Address, height int64, val uint64) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	heightMapping, ok := mr.register[addr]
	if !ok {
		heightMapping = make(map[int64]uint64, 0)
	}
	heightMapping[height] = val
}

func (mr *mockRegister) Balance(addr common.Address, height int64) uint64 {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	heightMapping, ok := mr.register[addr]
	if !ok {
		return 0
	}
	return heightMapping[height]
}

func TestChainId(t *testing.T) {
	db := dbm.NewMemDB()
	defer func() {
		require.NoError(t, db.Close())
	}()
	blockStore := store.NewBlockStore(db)
	engine.NewEthAPI(blockStore, newMockRegister(), "chainid", server.DefaultLogger())
}
