package engine

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type Node interface {
	LastBlockHeight() int64
	// The latest unsafe block hash
	//
	// The latest unsafe block refers to sealed blocks, not the one that's being built on
	HeadBlockHash() common.Hash
	CommitBlock() error
	UpdateLabel(label eth.BlockLabel, hash common.Hash) error
	Rollback(head, safe, finalized *eetypes.Block) error
}

type EngineAPI struct {
	node         Node
	blockStore   store.BlockStoreReader
	payloadStore payloadstore.PayloadStore
	lock         sync.RWMutex
}

func NewEngineAPI(node Node, blockStore store.BlockStoreReader, payloadStore payloadstore.PayloadStore) *EngineAPI {
	return &EngineAPI{
		node:         node,
		blockStore:   blockStore,
		payloadStore: payloadStore,
	}
}

func (e *EngineAPI) rollback(head *eetypes.Block, safeHash, finalizedHash common.Hash) error {
	getBlock := func(label eth.BlockLabel, hash common.Hash) *eetypes.Block {
		if hash != (common.Hash{}) {
			return e.blockStore.BlockByHash(hash)
		}
		return e.blockStore.BlockByLabel(label)
	}
	safe := getBlock(eth.Safe, safeHash)
	finalized := getBlock(eth.Finalized, finalizedHash)
	return e.node.Rollback(head, safe, finalized)
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

	// We expect to have the head block in the db already. What if we don't? How will we receive it?
	headBlock := e.blockStore.BlockByHash(fcs.HeadBlockHash)
	if headBlock == nil {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("head block: %w", ethereum.NotFound))
	}

	// update labeled blocks

	reorg := false
	// When OpNode issues a FCU with a head block that's different than App's view, it means a reorg happened.
	// In this case, we need to rollback App and BlockStore to the head block's height-1.
	if headBlock.Height() != e.node.LastBlockHeight() {
		// NOTE: with a single centralized sequencer, a reorg can only take us backwards.
		if err := e.rollback(headBlock, fcs.SafeBlockHash, fcs.FinalizedBlockHash); err != nil {
			return nil, engine.InvalidForkChoiceState.With(err)
		}
		reorg = true
	}

	// TODO I don't think using InvalidForkChoiceState everywhere makes sense.

	// update canonical block head
	if err := e.node.UpdateLabel(eth.Unsafe, fcs.HeadBlockHash); err != nil {
		return nil, engine.InvalidForkChoiceState.With(err)
	}

	if fcs.SafeBlockHash != (common.Hash{}) {
		if err := e.node.UpdateLabel(eth.Safe, fcs.SafeBlockHash); err != nil {
			return nil, engine.InvalidForkChoiceState.With(err)
		}
	}

	// update finalized block head
	if fcs.FinalizedBlockHash != (common.Hash{}) {
		if err := e.node.UpdateLabel(eth.Finalized, fcs.FinalizedBlockHash); err != nil {
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
		if err := e.payloadStore.Add(payload); err != nil {
			return nil, engine.InvalidPayloadAttributes.With(err) // TODO better error
		}
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
		return payload.Valid(payloadId), nil
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

// OpNode sequencer calls this API to seal a new block
func (e *EngineAPI) GetPayloadV3(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	payload, ok := e.payloadStore.Get(payloadID)
	if !ok {
		return nil, eetypes.UnknownPayload
	}
	if payload != e.payloadStore.Current() {
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
		log.Panicf("failed to commit block: %v", err)
	}

	return payload.ToExecutionPayloadEnvelope(e.node.HeadBlockHash()), nil
}

func (e *EngineAPI) NewPayloadV1(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV2(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV3(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.blockStore.BlockByHash(payload.BlockHash) == nil {
		return &eth.PayloadStatusV1{Status: eth.ExecutionInvalidBlockHash}, engine.InvalidParams.With(errors.New("block not found"))
	}
	headBlockHash := e.node.HeadBlockHash()
	return &eth.PayloadStatusV1{
		Status:          eth.ExecutionValid,
		LatestValidHash: &headBlockHash,
	}, nil
}
