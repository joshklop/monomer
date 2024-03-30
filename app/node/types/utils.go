package types

import (
	"crypto/sha256"
	"encoding/binary"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type Payload struct {
	Timestamp             uint64
	PrevRandao            [32]byte
	SuggestedFeeRecipient common.Address
	Withdrawals           *ethtypes.Withdrawals
	NoTxPool              bool
	GasLimit              uint64
	ParentBeaconBlockRoot *common.Hash
	ParentHash            common.Hash
	Height                int64
	Transactions          []hexutil.Bytes
	CosmosTxs             bfttypes.Txs
	id                    *engine.PayloadID
}

// ID returns a PaylodID (a hash) from a PayloadAttributes when it's applied to a head block.
// Hashing does not conform to go-ethereum/miner/payload_building.go
// PayloadID is only caclulated once, and cached for future calls.
func (p *Payload) ID() *engine.PayloadID {
	if p.id != nil {
		return p.id
	}

	hasher := sha256.New()
	hasher.Write(p.ParentHash[:])
	binary.Write(hasher, binary.BigEndian, p.Timestamp)
	hasher.Write(p.PrevRandao[:])
	hasher.Write(p.SuggestedFeeRecipient[:])
	binary.Write(hasher, binary.BigEndian, p.GasLimit)

	if p.NoTxPool || len(p.Transactions) == 0 {
		binary.Write(hasher, binary.BigEndian, p.NoTxPool)
		binary.Write(hasher, binary.BigEndian, uint64(len(p.Transactions)))
		for _, txData := range p.CosmosTxs {
			hasher.Write(txData)
		}
	}

	var out engine.PayloadID
	copy(out[:], hasher.Sum(nil)[:8])
	p.id = &out
	return &out
}

// ToExecutionPayloadEnvelope converts a Payload to an ExecutionPayload.
func (p *Payload) ToExecutionPayloadEnvelope(blockHash common.Hash) *eth.ExecutionPayloadEnvelope {
	return &eth.ExecutionPayloadEnvelope{
		ExecutionPayload: &eth.ExecutionPayload{
			ParentHash:   p.ParentHash,
			BlockNumber:  hexutil.Uint64(p.Height),
			BlockHash:    blockHash,
			FeeRecipient: p.SuggestedFeeRecipient,
			Timestamp:    hexutil.Uint64(p.Timestamp),
			PrevRandao:   p.PrevRandao,
			Withdrawals:  p.Withdrawals,
			Transactions: p.Transactions,
			GasLimit:     hexutil.Uint64(p.GasLimit),
		},
	}
}

// ValidForkchoiceUpdateResult returns a valid ForkchoiceUpdateResult with given head block hash.
func ValidForkchoiceUpdateResult(headBlockHash *common.Hash, id *engine.PayloadID) *eth.ForkchoiceUpdatedResult {
	return &eth.ForkchoiceUpdatedResult{
		PayloadStatus: eth.PayloadStatusV1{
			Status:          eth.ExecutionValid,
			LatestValidHash: headBlockHash,
		},
		PayloadID: id,
	}
}
