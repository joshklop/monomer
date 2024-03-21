package engine

import (
	"errors"
	"fmt"
	"log"
	"sync"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/payloadstore"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/builder"
)

type BlockStore interface {
	store.BlockStoreReader
	// TODO have an interface in the block store for this, so we don't need to copy it here
	UpdateLabel(label eth.BlockLabel, hash common.Hash) error
}

// EngineAPI implements the Engine API. It assumes it is the sole block proposer.
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
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	// TODO should this be called after Ecotone?
	return e.ForkchoiceUpdatedV3(fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV2(
	fcs eth.ForkchoiceState,
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	// TODO should this be called after Ecotone?
	return e.ForkchoiceUpdatedV3(fcs, pa)
}

func (e *EngineAPI) ForkchoiceUpdatedV3(
	fcs eth.ForkchoiceState,
	pa *eth.PayloadAttributes,
) (*eth.ForkchoiceUpdatedResult, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	// OP spec:
	//   - headBlockHash: block hash of the head of the canonical chain. Labeled "unsafe" in user JSON-RPC.
	//     Nodes may apply L2 blocks out of band ahead of time, and then reorg when L1 data conflicts.
	//   - safeBlockHash: block hash of the canonical chain, derived from L1 data, unlikely to reorg.
	//   - finalizedBlockHash: irreversible block hash, matches lower boundary of the dispute period.

	headBlock := e.blockStore.BlockByHash(fcs.HeadBlockHash)

	// Engine API spec:
	//   Before updating the forkchoice state, client software MUST ensure the validity of the payload referenced by forkchoiceState.headBlockHash...
	// Because we assume we're the only proposer, this is equivalent to checking if the head block is present in the block store.
	if headBlock == nil {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("head block not found"))
	}

	// Engine API spec:
	//   Client software MUST return -38002: Invalid forkchoice state error if the payload referenced by forkchoiceState.headBlockHash
	//   is VALID and a payload referenced by either forkchoiceState.finalizedBlockHash or forkchoiceState.safeBlockHash does not
	//   belong to the chain defined by forkchoiceState.headBlockHash.
	if safeBlock := e.blockStore.BlockByHash(fcs.SafeBlockHash); safeBlock == nil {
		return nil, engine.InvalidPayloadAttributes.With(errors.New("safe block not found"))
	} else if safeBlock.Header.Height > headBlock.Header.Height {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("safe block at height %d comes after head block at height %d", safeBlock.Header.Height, headBlock.Header.Height))
	}
	if finalizedBlock := e.blockStore.BlockByHash(fcs.FinalizedBlockHash); finalizedBlock == nil {
		return nil, engine.InvalidPayloadAttributes.With(errors.New("finalized block not found"))
	} else if finalizedBlock.Header.Height > headBlock.Header.Height {
		return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("finalized block at height %d comes after head block at height %d", finalizedBlock.Header.Height, headBlock.Header.Height))
	}

	// Engine API spec:
	//   Client software MAY skip an update of the forkchoice state and MUST NOT begin a payload build process if forkchoiceState.headBlockHash references an ancestor of the head of canonical chain.
	// This part of the spec does not apply to us.
	// Because we assume we're the sole proposer, the CL should only give us a past block head hash when L1 reorgs.
	// TODO Is reorg handling in the Engine API discussed in the OP Execution Engine spec?
	if headBlock.Header.Height < e.blockStore.HeadBlock().Header.Height {
		if err := e.builder.Rollback(fcs.HeadBlockHash, fcs.SafeBlockHash, fcs.FinalizedBlockHash); err != nil {
			return nil, engine.GenericServerError.With(fmt.Errorf("rollback: %v", err))
		}
		if err := e.payloadStore.RollbackToHeight(headBlock.Header.Height); err != nil {
			return nil, engine.InvalidForkChoiceState.With(fmt.Errorf("roll back payload store: %v", err))
		}
	}

	// Update block labels.
	// Since we know all of the hashes exist in the block store already, we can ignore the errors.
	e.blockStore.UpdateLabel(eth.Unsafe, fcs.HeadBlockHash)
	e.blockStore.UpdateLabel(eth.Safe, fcs.SafeBlockHash)
	e.blockStore.UpdateLabel(eth.Finalized, fcs.FinalizedBlockHash)

	if pa != nil {
		// Ethereum execution specs:
		//   https://github.com/ethereum/execution-specs/blob/119208cf1a13d5002074bcee3b8ea4ef096eeb0d/src/ethereum/shanghai/fork.py#L298
		if headTime := e.blockStore.HeadBlock().Header.Time; uint64(pa.Timestamp) <= headTime {
			return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("timestamp too small: parent timestamp %d, got %d", headTime, pa.Timestamp))
		}

		// Docs on OP PayloadAttributes struct:
		//   Withdrawals... should be nil or empty depending on Shanghai enablement
		// We assume Shanghai is enabled on L1.
		if pa.Withdrawals == nil {
			return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("withdrawals: want empty, got null"))
		} else if len(*pa.Withdrawals) > 0 {
			return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("withdrawals: want empty, got non-empty"))
		}

		// OP Spec:
		//   The gasLimit is optional w.r.t. compatibility with L1, but required when used as rollup.
		//   This field overrides the gas limit used during block-building. If not specified as rollup, a STATUS_INVALID is returned.
		// Monomer is always used as a rollup.
		// I do not know how to reconcile the above with:
		// Engine API spec:
		//   Client software MUST respond to this method call in the following way: ...
		//     [InvalidPayloadAttributes] if the payload is deemed VALID and forkchoiceState has been applied successfully, but no build process has been started due to invalid payloadAttributes.
		// STATUS_INVALID is only for applying the head block payload (which doesn't really apply to us anyway since we would have already built, committed, and proposed the head block).
		// OP-Geth returns InvalidParams.
		if pa.GasLimit == nil {
			return nil, engine.InvalidPayloadAttributes.With(errors.New("gas limit not provided"))
		}

		// OP Spec:
		//   Starting at Ecotone, the parentBeaconBlockRoot must be set to the L1 origin parentBeaconBlockRoot, or a zero bytes32 if the Dencun functionality with parentBeaconBlockRoot is not active on L1.
		// We assume Dencun is enabled on L1.
		if pa.ParentBeaconBlockRoot == nil {
			return nil, engine.InvalidPayloadAttributes.With(errors.New("parent beacon block root not provided"))
		}

		// OP Spec:
		//   If the transactions field is present, the engine must execute the transactions in order and return STATUS_INVALID if there is an error processing the transactions.
		//   It must return STATUS_VALID if all of the transactions could be executed without error.
		// TODO
		//   - may require modifying the builder to BeginBlock and DeliverTx separately from the rest of the build process.

		// Engine API spec:
		//   Client software MUST begin a payload build process building on top of forkchoiceState.headBlockHash and identified via buildProcessId value if payloadAttributes
		//   is not null and the forkchoice state has been updated successfully.
		payload := eetypes.NewPayload(pa, fcs.HeadBlockHash, e.blockStore.HeadBlock().Header.Height+1)
		payloadId, err := payload.GetPayloadID()
		if err != nil {
			return nil, engine.InvalidPayloadAttributes.With(fmt.Errorf("get payload id: %v", err))
		}
		if err := e.payloadStore.Add(payload); err != nil {
			return nil, engine.GenericServerError.With(fmt.Errorf("add payload to store: %v", err))
		}

		// Engine API spec:
		//   latestValidHash: ... the hash of the most recent valid block in the branch defined by payload and its ancestors.
		// Recall that "payload" refers to the most recent block appended to the canonical chain, not the payload attributes.
		return eetypes.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, payloadId), nil
	}

	// Engine API spec:
	//   `payloadId: null`... if the payload is deemed VALID and a build process hasn't been started.
	return eetypes.ValidForkchoiceUpdateResult(&fcs.HeadBlockHash, nil), nil
}

func (e *EngineAPI) GetPayloadV1(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	// TODO should this be called after Ecotone?
	return e.GetPayloadV3(payloadID)
}

func (e *EngineAPI) GetPayloadV2(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	// TODO should this be called after Ecotone?
	return e.GetPayloadV3(payloadID)
}

// GetPayloadV3 seals a payload that is currently being built (i.e. was introduced in the PayloadAttributes from a previous ForkchoiceUpdated call).
func (e *EngineAPI) GetPayloadV3(payloadID eetypes.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	payload := e.payloadStore.Current()
	if currentID, err := payload.GetPayloadID(); err != nil {
		return nil, engine.GenericServerError.With(fmt.Errorf("calculate id of current payload: %v", err))
	} else if payloadID != *currentID {
		return nil, engine.InvalidParams.With(errors.New("payload is not current"))
	}

	// TODO: handle time slot based block production
	// for now assume block is sealed by this call
	// TODO what does the above todo mean?
	if err := e.builder.Build(&builder.Payload{
		Transactions: func() bfttypes.Txs {
			var txs bfttypes.Txs
			for _, tx := range payload.Attrs.Transactions {
				txs = append(txs, bfttypes.Tx(tx))
			}
			// TODO we want to make a tx compatible with the rollup module.
			//   Ideally we solve the root of the problem and have already delivered the rollup txs in ForkchoiceUpdated.
			//   Then we can remove the Transactions field from builder.Payload.
			return txs
		}(),
		// We know it is non-nil from payload validation.
		// TODO make payloadstore store `builder.Payload`s.
		GasLimit: uint64(*payload.Attrs.GasLimit),
		Timestamp: uint64(payload.Attrs.Timestamp),
		// TODO don't ignore the NoTxPool option.
	}); err != nil {
		log.Panicf("failed to commit block: %v", err) // TODO error handling. An error here is potentially a big problem.
	}

	return payload.ToExecutionPayloadEnvelope(e.blockStore.HeadBlock().Hash()), nil
}

func (e *EngineAPI) NewPayloadV1(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	// TODO should this be called after Ecotone?
	return e.NewPayloadV3(payload)
}

func (e *EngineAPI) NewPayloadV2(payload eth.ExecutionPayload) (*eth.PayloadStatusV1, error) {
	// TODO should this be called after Ecotone?
	return e.NewPayloadV3(payload)
}

// NewPayloadV3 ensures the payload's block hash is present in the block store.
// TODO will this ever be called if we are the sole block proposer?
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