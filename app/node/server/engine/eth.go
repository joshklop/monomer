package engine

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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
	return adaptBlock(b, inclTx), nil
}

func (e *EthAPI) GetBlockByNumber(id any, inclTx bool) (map[string]any, error) {
	b := e.blockByID(id)
	if b == nil {
		return nil, errors.New("block not found") // TODO: do we need a special error?
	}
	return adaptBlock(b, inclTx), nil
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

// This trick is played by the eth rpc server too. Instead of constructing
// an actual eth block, simply create a map with the right keys so the client
// can unmarshal it into a block
func adaptBlock(b *eetypes.Block, inclTx bool) map[string]any {
	result := map[string]any{
		// These are the ones that make sense to polymer.
		"parentHash": b.ParentBlockHash,
		"stateRoot":  common.BytesToHash(b.Header.AppHash),
		"number":     (*hexutil.Big)(big.NewInt(b.Height())),
		"gasLimit":   b.GasLimit,
		"mixHash":    b.PrevRandao,
		"timestamp":  hexutil.Uint64(b.Header.Time),
		"hash":       b.Hash(),

		// these are required fields that need to be part of the header or
		// the eth client will complain during unmarshalling
		"sha3Uncles":      ethtypes.EmptyUncleHash,
		"receiptsRoot":    ethtypes.EmptyReceiptsHash,
		"baseFeePerGas":   (*hexutil.Big)(common.Big0),
		"difficulty":      (*hexutil.Big)(common.Big0),
		"extraData":       make([]byte, 0),
		"gasUsed":         hexutil.Uint64(0),
		"logsBloom":       ethtypes.Bloom(make([]byte, ethtypes.BloomByteLength)),
		"withdrawalsRoot": ethtypes.EmptyWithdrawalsHash,
		"withdrawals":     b.Withdrawals,
	}

	txs, root := b.Transactions()
	if inclTx {
		result["transactionsRoot"] = root
		result["transactions"] = txs
	} else {
		result["transactionsRoot"] = root
	}
	return result
}
