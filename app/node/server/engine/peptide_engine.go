package engine

import (
	"fmt"
	"log"
	"math/big"
	"strconv"
	"sync"

	"github.com/armon/go-metrics"
	"github.com/cosmos/cosmos-sdk/telemetry"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
)

type (
	Hash  = eetypes.Hash
	Block = eetypes.Block
)

type Node interface {
	LastBlockHeight() int64
	// SavePayload saves the payload by its ID if it's not already in payload cache.
	// Also update the latest Payload if this is a new payload
	// The latest unsafe block hash
	//
	// The latest unsafe block refers to sealed blocks, not the one that's being built on
	HeadBlockHash() Hash
	CommitBlock() error
	// GetETH returns the wrapped ETH balance in Wei of the given EVM address.
	GetETH(address common.Address, height int64) (*big.Int, error)

	GetBlock(id any) (*Block, error)
	UpdateLabel(label eth.BlockLabel, hash Hash) error

	Rollback(head, safe, finalized *Block) error
}

type EngineAPI struct {
	node   Node
	payloadStore payloadstore.PayloadStore
	logger server.Logger
	lock   sync.RWMutex
}

func NewEngineAPI(node Node, payloadStore payloadstore.PayloadStore, logger server.Logger) *EngineAPI {
	return &EngineAPI{
		node: node,
		payloadStore: payloadStore,
		logger: logger,
	}
}

func (e *EngineAPI) rollback(head *Block, safeHash, finalizedHash eetypes.Hash) error {
	e.logger.Debug("engineAPIserver.rollback", "head", head.Height(), "safe", safeHash, "finalized", finalizedHash)

	getId := func(label string, hash eetypes.Hash) any {
		if hash != eetypes.ZeroHash {
			return hash.Bytes()
		}
		return label
	}
	safe, err := e.node.GetBlock(getId(eth.Safe, safeHash))
	if err != nil {
		return err
	}
	finalized, err := e.node.GetBlock(getId(eth.Finalized, finalizedHash))
	if err != nil {
		return err
	}
	return e.node.Rollback(head, safe, finalized)
}

func (e *EngineAPI) ForkchoiceUpdatedV1(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	e.logger.Debug("trying: ForkchoiceUpdatedV1")
	return e.ForkchoiceUpdatedV3(fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV2(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	e.logger.Debug("trying: ForkchoiceUpdatedV2")
	return e.ForkchoiceUpdatedV3(fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV3(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	e.logger.Debug("trying: ForkchoiceUpdatedV3",
		"appHeight", e.node.LastBlockHeight()+1,
		"unsafe", fcs.HeadBlockHash.Hex(),
		"safe", fcs.SafeBlockHash.Hex(),
		"finalized", fcs.FinalizedBlockHash.Hex(),
		"attr", eetypes.HasPayloadAttributes(&pa),
	)
	e.lock.Lock()
	defer e.lock.Unlock()

	headBlock, err := e.node.GetBlock(fcs.HeadBlockHash.Bytes())
	if err != nil {
		e.logger.Error("failed to get headBlock", "headBlockHash", fcs.HeadBlockHash.Hex(), "err", err)
		return nil, engine.InvalidForkChoiceState.With(err)
	}

	e.logger.Debug("ForkchoiceUpdatedV3",
		"appHeight", e.node.LastBlockHeight()+1,
		"fcu.unsafe.height", headBlock.Height(),
	)

	defer telemetry.IncrCounter(1, "query", "ForkchoiceUpdated")

	if eetypes.IsForkchoiceStateEmpty(&fcs) {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("forkchoice state is empty"))
	}

	// update labeled blocks

	reorg := false
	// When OpNode issues a FCU with a head block that's different than App's view, it means a reorg happened.
	// In this case, we need to rollback App and BlockStore to the head block's height-1.
	if headBlock.Height() != e.node.LastBlockHeight() {
		e.logger.Info("block head does not match the last sealed block [reorg from OpNode]", "head_height", headBlock.Height(), "app_height", e.node.LastBlockHeight())
		if err := e.rollback(headBlock, fcs.SafeBlockHash, fcs.FinalizedBlockHash); err != nil {
			e.logger.Error("rollback failed: %w", err)
			return nil, engine.InvalidForkChoiceState.With(err)
		}
		e.logger.Info("rollback succeeded", "head_height", headBlock.Height(), "app_height", e.node.LastBlockHeight())
		reorg = true
	}

	// update canonical block head
	e.logger.Info("updating unsafe/latest block", "hash", fcs.SafeBlockHash, "height", headBlock.Height())
	e.node.UpdateLabel(eth.Unsafe, fcs.HeadBlockHash)

	if fcs.SafeBlockHash != eetypes.ZeroHash {
		e.logger.Info("updating safe block", "hash", fcs.SafeBlockHash)
		if err := e.node.UpdateLabel(eth.Safe, fcs.SafeBlockHash); err != nil {
			e.logger.Error("invalid safe head", "err", err)
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	// update finalized block head
	if fcs.FinalizedBlockHash != eetypes.ZeroHash {
		e.logger.Info("updating finalized block", "hash", fcs.FinalizedBlockHash)
		if err := e.node.UpdateLabel(eth.Finalized, fcs.FinalizedBlockHash); err != nil {
			e.logger.Error("invalid finalized head", "err", err)
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	// OpNode providing a new payload with reorg
	if reorg {
		payload := eetypes.NewPayload(&pa, fcs.HeadBlockHash, e.node.LastBlockHeight()+1)
		payloadId, err := payload.GetPayloadID()
		if err != nil {
			return nil, engine.InvalidPayloadAttributes.With(err)
		}
		// TODO: handle error of SavePayload
		e.payloadStore.Add(payload)
		e.logger.Info("engine reorg payload", "payload_id", payloadId, "payload_head_block_hash", fcs.HeadBlockHash, "store_head_block_hash", e.node.HeadBlockHash())
		// TODO: use one method for both cases: payload.Valid()
		return eetypes.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, payloadId), nil
	}

	// start new payload mode
	if eetypes.HasPayloadAttributes(&pa) {
		// TODO check for invalid txs in pa
		payload := eetypes.NewPayload(&pa, fcs.HeadBlockHash, e.node.LastBlockHeight()+1)
		payloadId, err := payload.GetPayloadID()
		if err != nil {
			return nil, engine.InvalidPayloadAttributes.With(err)
		}
		e.payloadStore.Get(*payloadId)
		e.logger.Info("engine saving new payload", "payload_id", payloadId, "payload_head_block_hash", fcs.HeadBlockHash, "store_head_block_hash", e.node.HeadBlockHash(), "headBlockHeight", headBlock.Height())
		return payload.Valid(payloadId), nil
	}

	// OpNode providing an existing payload, which only updates the head latest/unsafe block pointer
	// after reboot, in-mem payload cache is lost, causing OpNode failed to find Payload
	e.logger.Info("engine updating head block with existing payload", "hash", fcs.HeadBlockHash, "headBlockHeight", headBlock.Height())
	return eetypes.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, nil), nil
}

func (e *EngineAPI) GetPayloadV1(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.logger.Debug("GetPayloadV1", "payload_id", payloadID)
	return e.GetPayloadV3(payloadID)
}

func (e *EngineAPI) GetPayloadV2(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.logger.Debug("GetPayloadV2", "payload_id", payloadID)
	return e.GetPayloadV3(payloadID)
}

// OpNode sequencer calls this API to seal a new block
func (e *EngineAPI) GetPayloadV3(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	newBlockHeight := e.node.LastBlockHeight() + 1
	e.logger.Debug("GetPayloadV3", "payload_id", payloadID, "newBlockHeight", newBlockHeight)

	defer telemetry.IncrCounter(1, "query", "GetPayload")

	payload, ok := e.payloadStore.Get(payloadID)
	if !ok {
		return nil, eetypes.UnknownPayload
	}
	if payload != e.payloadStore.Current() {
		e.logger.Error("payload is not current", "payload_id", payloadID, "newBlockHeight", newBlockHeight)
		return nil, engine.InvalidParams.With(fmt.Errorf("payload is not current"))
	}

	// e.mutex.Lock()
	// defer e.mutex.Unlock()

	// e.debugL1UserTxs(payload.Attrs.Transactions, "EngineGetPayload")

	// TODO: handle time slot based block production
	// for now assume block is sealed by this call
	err := e.node.CommitBlock()
	// TODO error handling
	if err != nil {
		e.logger.Error("failed to commit block", "err", err)
		log.Panicf("failed to commit block: %v", err)
	}

	return payload.ToExecutionPayloadEnvelope(e.node.HeadBlockHash()), nil
}

func (e *EngineAPI) NewPayloadV1(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	e.logger.Debug("trying: NewPayloadV1", "payload.ID", payload.ID(), "blockHash", payload.BlockHash.Hex(), "height", e.node.LastBlockHeight()+1)
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV2(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	e.logger.Debug("trying: NewPayloadV2", "payload.ID", payload.ID(), "blockHash", payload.BlockHash.Hex(), "height", e.node.LastBlockHeight()+1)
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV3(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	e.logger.Debug("trying: NewPayloadV3", "payload.ID", payload.ID(), "blockHash", payload.BlockHash.Hex(), "height", e.node.LastBlockHeight()+1)
	e.lock.Lock()
	defer e.lock.Unlock()
	defer telemetry.IncrCounter(1, "query", "NewPayload")

	e.logger.Debug("NewPayloadV3", "payload.ID", payload.ID(), "blockHash", payload.BlockHash.Hex(), "height", e.node.LastBlockHeight()+1)

	if _, err := e.node.GetBlock(payload.BlockHash.Bytes()); err != nil {
		e.logger.Error("Engine.NewPayload: failed to get block", "blockHash", payload.BlockHash.Hex(), "err", err)
		return &eth.PayloadStatusV1{Status: eth.ExecutionInvalidBlockHash},
			engine.InvalidParams.With(err)
	}
	headBlockHash := e.node.HeadBlockHash()
	return &eth.PayloadStatusV1{
		Status:          eth.ExecutionValid,
		LatestValidHash: &headBlockHash,
	}, nil
}

// ------------------------------------------------------------------------------------------------------------------

type EthAPI struct {
	node   Node
	logger server.Logger
	chainID string
}

func NewEthAPI(node Node, chainID string, logger server.Logger) *EthAPI {
	return &EthAPI{
		node:   node,
		chainID: chainID,
		logger: logger,
	}
}

func (e *EthAPI) GetProof(address common.Address, storage []Hash, blockTag string) (*eth.AccountResult, error) {
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

	b, err := e.node.GetBlock(id)
	if err != nil {
		return hexutil.Big{}, err
	}

	balance, err := e.node.GetETH(address, b.Height())
	if err != nil {
		err = fmt.Errorf("failed to get balance for address %s at block height %d, %w", address, b.Height(), err)
		return hexutil.Big{}, err
	}
	return (hexutil.Big)(*balance), nil
}

func (e *EthAPI) GetBlockByHash(hash Hash, inclTx bool) (map[string]any, error) {
	e.logger.Debug("GetBlockByHash", "hash", hash.Hex(), "inclTx", inclTx)
	telemetry.IncrCounterWithLabels([]string{"query", "GetBlockByHash"}, 1, []metrics.Label{telemetry.NewLabel("inclTx", strconv.FormatBool(inclTx))})

	b, err := e.node.GetBlock(hash.Bytes())
	if err != nil {
		return nil, err
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) GetBlockByNumber(id any, inclTx bool) (map[string]any, error) {
	telemetry.IncrCounterWithLabels([]string{"query", "GetBlockByNumber"}, 1, []metrics.Label{telemetry.NewLabel("inclTx", strconv.FormatBool(inclTx))})

	b, err := e.node.GetBlock(id)
	// OpNode needs a NotFound error to trigger Engine reset
	if err != nil || b == nil {
		e.logger.Debug("GetBlockByNumber", "id", id, "inclTx", inclTx, "found", false)
		// non-nil err translates to a TempErr in OpNode;
		// What we need is a nil err/block, which translates to a NotFound error in OpNode
		return nil, nil
	}
	e.logger.Debug("GetBlockByNumber", "id", id, "inclTx", inclTx, "found", true)
	return b.ToEthLikeBlock(inclTx), nil
}

// ------------------------------------------------------------------------------------------------------------------

type PeptideAPI struct {
	node   Node
	logger server.Logger
}

func NewPeptideAPI(node Node, logger server.Logger) *PeptideAPI {
	return &PeptideAPI{
		node:   node,
		logger: logger,
	}
}

func (e *PeptideAPI) GetBlock(id any) (*Block, error) {
	e.logger.Debug("GetBlock", "id", id)
	telemetry.IncrCounter(1, "query", "GetBlock")

	block, err := e.node.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return block, nil
}

// ------------------------------------------------------------------------------------------------------------------

// admin API
type AdminAPI struct {
	node   Node
	logger server.Logger
}

func NewAdminAPI(node Node, logger server.Logger) *AdminAPI {
	return &AdminAPI{
		node:   node,
		logger: logger,
	}
}

func (e *AdminAPI) Rollback(headHeight, safeHeight, finalizedHeight int64) error {
	e.logger.Debug("Rollback", "head", headHeight, "safe", safeHeight, "finalized", finalizedHeight)
	head, err := e.node.GetBlock(headHeight)
	if err != nil {
		return err
	}
	safe, err := e.node.GetBlock(safeHeight)
	if err != nil {
		return err
	}
	finalized, err := e.node.GetBlock(finalizedHeight)
	if err != nil {
		return err
	}

	return e.node.Rollback(head, safe, finalized)
}
