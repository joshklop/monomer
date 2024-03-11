package engine

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type WeiRegister interface {
	// Balance returns a non-nil balance and a nil error, or a nil balance and a non-nil error.
	Balance(address common.Address, height int64) (*big.Int, error)
}

type EthAPI struct {
	blockStore store.BlockStoreReader
	register   WeiRegister
	chainID    *hexutil.Big
}

func NewEthAPI(blockStore store.BlockStoreReader, register WeiRegister, chainID *hexutil.Big) *EthAPI {
	return &EthAPI{
		blockStore: blockStore,
		register:   register,
		chainID:    chainID,
	}
}

func (e *EthAPI) GetProof(address common.Address, storage []common.Hash, blockTag string) (*eth.AccountResult, error) {
	// TODO
	return nil, nil
}

func (e *EthAPI) ChainId() *hexutil.Big {
	return e.chainID
}

func (e *EthAPI) GetBalance(address common.Address, id any) (*hexutil.Big, error) {
	b := e.blockByID(id)
	if b == nil {
		return nil, errors.New("block not found")
	}

	balance, err := e.register.Balance(address, b.Height())
	if err != nil {
		return nil, fmt.Errorf("get balance for address %s at height %d: %v", address, b.Header.Height, err)
	}
	return (*hexutil.Big)(balance), nil
}

func (e *EthAPI) GetBlockByHash(hash common.Hash, inclTx bool) (map[string]any, error) {
	b := e.blockStore.BlockByHash(hash)
	if b == nil {
		return nil, errors.New("block not found")
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) GetBlockByNumber(id any, inclTx bool) (map[string]any, error) {
	b := e.blockByID(id)
	if b == nil {
		return nil, errors.New("block not found") // TODO: do we need a special error?
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) blockByID(id any) *eetypes.Block {
	switch idT := id.(type) {
	case nil:
		return e.blockStore.BlockByLabel(eth.Unsafe)
	case int:
		return e.blockStore.BlockByNumber(int64(idT))
	case int64:
		return e.blockStore.BlockByNumber(idT)
	case eth.BlockLabel:
		return e.blockStore.BlockByLabel(idT)
	case string:
		return e.blockStore.BlockByLabel(eth.BlockLabel(idT))
	default:
		return nil
	}
}
