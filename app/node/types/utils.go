package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type PayloadID = engine.PayloadID

var (
	emptyPayloadID = PayloadID{}
	UnknownPayload = engine.UnknownPayload
)

// Verifies HeadBlockHash is empty
func IsForkchoiceStateEmpty(state *eth.ForkchoiceState) bool {
	return bytes.Equal(state.HeadBlockHash[:], ZeroHash[:])
}

func HasPayloadAttributes(attrs *eth.PayloadAttributes) bool {
	return attrs.Timestamp != 0 && attrs.Transactions != nil && len(attrs.Transactions) != 0
}

func IsPayloadIDEmpty(id PayloadID) bool {
	return bytes.Equal(id[:], emptyPayloadID[:])
}

type Payload struct {
	Attrs      *eth.PayloadAttributes
	ParentHash common.Hash
	Height     int64
	id         *PayloadID
	idErr      error
	cosmosTxs  []bfttypes.Tx
}

// NewPayload creates a new Payload from a PayloadAttributes and a head block hash.
func NewPayload(attrs *eth.PayloadAttributes, parentHash common.Hash, height int64) *Payload {
	return &Payload{
		Attrs:      attrs,
		ParentHash: parentHash,
		Height:     height,
	}
}

// GetPayloadID returns a PaylodID (a hash) from a PayloadAttributes when it's applied to a head block.
// Hashing conforms to go-ethereum/miner/payload_building.go
// PayloadID is only caclulated once, and cached for future calls.
func (p *Payload) GetPayloadID() (*PayloadID, error) {
	if p.id != nil {
		return p.id, nil
	}

	pa := p.Attrs
	if pa.GasLimit == nil {
		return nil, fmt.Errorf("missing GasLimit attribute")
	}

	hasher := sha256.New()
	hasher.Write(p.ParentHash[:])
	binary.Write(hasher, binary.BigEndian, pa.Timestamp)
	hasher.Write(pa.PrevRandao[:])
	hasher.Write(pa.SuggestedFeeRecipient[:])
	binary.Write(hasher, binary.BigEndian, *pa.GasLimit)

	if pa.NoTxPool || len(pa.Transactions) == 0 {
		binary.Write(hasher, binary.BigEndian, pa.NoTxPool)
		binary.Write(hasher, binary.BigEndian, uint64(len(pa.Transactions)))
		// txs must be deposit txs from L1, ie. eth txs
		for i, txData := range pa.Transactions {
			var tx ethtypes.Transaction
			if err := tx.UnmarshalBinary(txData); err != nil {
				p.idErr = fmt.Errorf("failed to unmarshal eth transaction with index: %d", i)
				p.id = nil
				return nil, p.idErr
			}
			hasher.Write(tx.Hash().Bytes())
		}
	}

	var out PayloadID
	copy(out[:], hasher.Sum(nil)[:8])
	p.id = &out
	return &out, nil
}

// Valid returns a valid ForkchoiceUpdateResult
func (p *Payload) Valid(id *PayloadID) *eth.ForkchoiceUpdatedResult {
	return ValidForkchoiceUpdateResult(&p.ParentHash, id)
}

// ToExecutionPayloadEnvelope converts a Payload to an ExecutionPayload.
func (p *Payload) ToExecutionPayloadEnvelope(blockHash common.Hash) *eth.ExecutionPayloadEnvelope {
	return &eth.ExecutionPayloadEnvelope{ExecutionPayload: &eth.ExecutionPayload{
		ParentHash:   p.ParentHash,
		BlockNumber:  hexutil.Uint64(p.Height),
		BlockHash:    blockHash,
		FeeRecipient: p.Attrs.SuggestedFeeRecipient,
		Timestamp:    p.Attrs.Timestamp,
		PrevRandao:   p.Attrs.PrevRandao,
		// add cosmos txs too?
		Withdrawals:  p.Attrs.Withdrawals,
		Transactions: p.Attrs.Transactions,
		GasLimit:     *p.Attrs.GasLimit,
	}}
}

// ValidForkchoiceUpdateResult returns a valid ForkchoiceUpdateResult with given head block hash.
func ValidForkchoiceUpdateResult(headBlockHash *common.Hash, id *PayloadID) *eth.ForkchoiceUpdatedResult {
	return &eth.ForkchoiceUpdatedResult{
		PayloadStatus: eth.PayloadStatusV1{
			Status:          eth.ExecutionValid,
			LatestValidHash: headBlockHash,
		},
		PayloadID: id,
	}
}
