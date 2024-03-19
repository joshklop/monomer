package engine

import (
	"errors"
	"fmt"
	"log"
	"sync"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/builder"
)

type BlockStore interface {
	store.BlockStoreReader
	UpdateLabel(label eth.BlockLabel, hash common.Hash) error
}

type EngineAPI struct {
	builder      *builder.Builder
	blockStore   BlockStore
	payloadStore payloadstore.PayloadStore
	lock         sync.RWMutex
}

func NewEngineAPI(builder *builder.Builder, blockStore BlockStore) *EngineAPI {
	return &EngineAPI{
		blockStore:   blockStore,
		builder:      builder,
		payloadStore: payloadstore.NewPayloadStore(),
	}
}

func (e *EngineAPI) ForkchoiceUpdatedV1(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	return e.ForkchoiceUpdatedV3(fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV2(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	return e.ForkchoiceUpdatedV3(fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV3(
	fcs eth.ForkchoiceState,
	pa eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	headBlock := e.blockStore.BlockByHash(fcs.HeadBlockHash)
	if headBlock == nil {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("head block: %w", ethereum.NotFound))
	}

	// update labeled blocks

	reorg := false
	// When OpNode issues a FCU with a head block that's different than App's view, it means a reorg happened.
	// In this case, we need to rollback App and BlockStore to the head block's height-1.
	if headBlock.Header.Height < e.blockStore.HeadBlock().Header.Height {
		if err := e.builder.Rollback(fcs.HeadBlockHash, fcs.SafeBlockHash, fcs.FinalizedBlockHash); err != nil {
			return nil, engine.InvalidForkChoiceState.With(err)
		}
		if err := e.payloadStore.RollbackToHeight(headBlock.Header.Height); err != nil {
			return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("roll back payload store: %v", err))
		}
		reorg = true
	}

	// TODO check errors with spec

	// update canonical block head
	if err := e.blockStore.UpdateLabel(eth.Unsafe, fcs.HeadBlockHash); err != nil {
		return nil, engine.InvalidForkChoiceState.With(err)
	}

	if fcs.SafeBlockHash != (common.Hash{}) {
		if err := e.blockStore.UpdateLabel(eth.Safe, fcs.SafeBlockHash); err != nil {
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	// update finalized block head
	if fcs.FinalizedBlockHash != (common.Hash{}) {
		if err := e.blockStore.UpdateLabel(eth.Finalized, fcs.FinalizedBlockHash); err != nil {
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	if eetypes.HasPayloadAttributes(&pa) {
		// TODO complete payload validation
		payload := eetypes.NewPayload(&pa, fcs.HeadBlockHash, e.blockStore.HeadBlock().Header.Height+1)
		payloadId, err := payload.GetPayloadID()
		if err != nil {
			return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("get payload id: %v", err))
		}
		if err := e.payloadStore.Add(payload); err != nil {
			return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("add payload to store: %v", err))
		}

		var headBlockHash common.Hash
		if reorg {
			headBlockHash = fcs.HeadBlockHash
		} else {
			headBlockHash = payload.ParentHash
		}
		return eetypes.ValidForkchoiceUpdateResult(&headBlockHash, payloadId), nil
	}

	// OpNode providing an existing payload, which only updates the head latest/unsafe block pointer
	// after reboot, in-mem payload cache is lost, causing OpNode failed to find Payload
	return eetypes.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, nil), nil
}

func (e *EngineAPI) GetPayloadV1(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	return e.GetPayloadV3(payloadID)
}

func (e *EngineAPI) GetPayloadV2(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	return e.GetPayloadV3(payloadID)
}

// GetPayloadV3 seals a payload that is currently being built (i.e. was introduced in the PayloadAttributes from a previous ForkchoiceUpdated call).
func (e *EngineAPI) GetPayloadV3(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	payload, ok := e.payloadStore.Get(payloadID)
	if !ok {
		return nil, eetypes.UnknownPayload
	} else if payload != e.payloadStore.Current() { // TODO should probably check by id, not address
		return nil, engine.InvalidParams.With(fmt.Errorf("payload is not current"))
	}

	// TODO: handle time slot based block production
	// for now assume block is sealed by this call
	if err := e.builder.Build(&builder.Payload{
		Transactions: func() bfttypes.Txs {
			var txs bfttypes.Txs
			for _, tx := range payload.Attrs.Transactions {
				txs = append(txs, bfttypes.Tx(tx))
			}
			// TODO we want to make a tx compatible with the rollup module.
			return txs
		}(),
		GasLimit: func() uint64 {
			if payload.Attrs.GasLimit == nil {
				return peptide.DefaultGasLimit
			}
			return uint64(*payload.Attrs.GasLimit)
		}(),
		Timestamp: uint64(payload.Attrs.Timestamp),
		// TODO Ignoring the NoTxPool option for now. Maybe other options as well?
	}); err != nil { 
		log.Panicf("failed to commit block: %v", err) // TODO error handling. this is potentially a big problem.
	}

	return payload.ToExecutionPayloadEnvelope(e.blockStore.HeadBlock().Hash()), nil
}

func (e *EngineAPI) NewPayloadV1(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV2(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	return e.NewPayloadV3(payload)
}

// NewPayloadV3 ensures the payload's block hash is present in the block store.
func (e *EngineAPI) NewPayloadV3(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.blockStore.BlockByHash(payload.BlockHash) == nil {
		return &eth.PayloadStatusV1{
			Status: eth.ExecutionInvalidBlockHash,
		}, engine.InvalidParams.With(errors.New("block not found"))
	}
	headBlockHash := e.blockStore.HeadBlock().Hash()
	return &eth.PayloadStatusV1{
		Status:          eth.ExecutionValid,
		LatestValidHash: &headBlockHash,
	}, nil
}
