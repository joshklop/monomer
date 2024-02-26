package engine

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/armon/go-metrics"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type WeiRegister interface {
	Balance(address common.Address, height int64) (*big.Int, error)
}

type EthAPI struct {
	blockStore store.BlockStoreReader
	logger     server.Logger
	register   WeiRegister
	chainID    string
}

func NewEthAPI(blockStore store.BlockStoreReader, register WeiRegister, chainID string, logger server.Logger) *EthAPI {
	return &EthAPI{
		blockStore: blockStore,
		register:   register,
		chainID:    chainID,
		logger:     logger,
	}
}

func (e *EthAPI) GetProof(address common.Address, storage []eetypes.Hash, blockTag string) (*eth.AccountResult, error) {
	e.logger.Debug("GetProof", "address", address, "storage", storage, "blockTag", blockTag)
	telemetry.IncrCounter(1, "query", "GetProof")

	return &eth.AccountResult{}, nil
}

func (e *EthAPI) ChainId() hexutil.Big {
	e.logger.Debug("ChainId")
	telemetry.IncrCounter(1, "query", "ChainId")

	chainID, ok := new(big.Int).SetString(e.chainID, 10)
	if !ok {
		panic("chain id is not numerical")
	}
	return (hexutil.Big)(*chainID)
}

// GetBalance returns wrapped Ethers balance on L2 chain
// - address: EVM address
// - blockNumber: a valid BlockLabel or hex encoded big.Int; default to latest/unsafe block
func (e *EthAPI) GetBalance(address common.Address, id any) (hexutil.Big, error) {
	e.logger.Debug("GetBalance", "address", address, "id", id)
	telemetry.IncrCounter(1, "query", "GetBalance")

	b := e.blockByID(id)
	if b == nil {
		e.logger.Debug("GetBlockByNumber", "id", id, "found", false)
		return hexutil.Big{}, errors.New("block not found")
	}

	balance, err := e.register.Balance(address, b.Height())
	if err != nil {
		err = fmt.Errorf("failed to get balance for address %s at block height %d, %w", address, b.Height(), err)
		return hexutil.Big{}, err
	}
	return (hexutil.Big)(*balance), nil
}

func (e *EthAPI) GetBlockByHash(hash eetypes.Hash, inclTx bool) (map[string]any, error) {
	e.logger.Debug("GetBlockByHash", "hash", hash.Hex(), "inclTx", inclTx)
	telemetry.IncrCounterWithLabels([]string{"query", "GetBlockByHash"}, 1, []metrics.Label{telemetry.NewLabel("inclTx", strconv.FormatBool(inclTx))})

	b := e.blockStore.BlockByHash(hash)
	if b == nil {
		return nil, errors.New("block not found")
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) GetBlockByNumber(id any, inclTx bool) (map[string]any, error) {
	telemetry.IncrCounterWithLabels([]string{"query", "GetBlockByNumber"}, 1, []metrics.Label{telemetry.NewLabel("inclTx", strconv.FormatBool(inclTx))})

	b := e.blockByID(id)
	// OpNode needs a NotFound error to trigger Engine reset
	if b == nil {
		e.logger.Debug("GetBlockByNumber", "id", id, "inclTx", inclTx, "found", false)
		// non-nil err translates to a TempErr in OpNode;
		// What we need is a nil err/block, which translates to a NotFound error in OpNode
		// TODO?
		return nil, nil
	}
	e.logger.Debug("GetBlockByNumber", "id", id, "inclTx", inclTx, "found", true)
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) blockByID(id any) *eetypes.Block {
	switch idT := id.(type) {
	case nil:
		return e.blockStore.BlockByLabel(eth.Unsafe)
	case int64:
		return e.blockStore.BlockByNumber(idT)
	case eth.BlockLabel:
		return e.blockStore.BlockByLabel(idT)
	}
	return nil
}
