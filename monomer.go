package monomer

import (
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"strconv"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

type Application interface {
	// Info/Query Connection
	Info(abcitypes.RequestInfo) abcitypes.ResponseInfo // Return application info
	Query(req abcitypes.RequestQuery) (res abcitypes.ResponseQuery)

	// Mempool Connection
	CheckTx(abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx // Validate a tx for the mempool

	// Consensus Connection
	InitChain(abcitypes.RequestInitChain) abcitypes.ResponseInitChain    // Initialize blockchain w validators/other info from CometBFT
	BeginBlock(abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock // Signals the beginning of a block
	DeliverTx(abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx    // Deliver a tx for full processing
	EndBlock(abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock       // Signals the end of a block, returns changes to the validator set
	Commit() abcitypes.ResponseCommit                                    // Commit the state and return the application Merkle root hash

	RollbackToHeight(uint64) error
}

type ChainID uint64

func (id ChainID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

func (id ChainID) HexBig() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).SetUint64(uint64(id)))
}

type Header struct {
	ChainID    ChainID     `json:"chain_id"`
	Height     int64       `json:"height"`
	Time       uint64      `json:"time"`
	ParentHash common.Hash `json:"parentHash"`
	// state after txs from the *previous* block
	AppHash  []byte      `json:"app_hash"`
	GasLimit uint64      `json:"gasLimit"`
	Hash     common.Hash `json:"hash"`
}

func (h *Header) ToComet() *bfttypes.Header {
	return &bfttypes.Header{
		ChainID: h.ChainID.String(),
		Height:  h.Height,
		Time:    time.Unix(int64(h.Time), 0),
		AppHash: h.AppHash,
	}
}

type Block struct {
	Header *Header      `json:"header"`
	Txs    bfttypes.Txs `json:"txs"`
}

// Hash returns a unique hash of the block, used as the block identifier
func (b *Block) Hash() common.Hash {
	if b.Header.Hash == (common.Hash{}) {
		// We exclude the tx commitment.
		// TODO better hashing technique than using Ethereum's.
		b.Header.Hash = (&ethtypes.Header{
			ParentHash:      b.Header.ParentHash,
			Root:            common.BytesToHash(b.Header.AppHash), // TODO actually take the keccak
			Number:          big.NewInt(b.Header.Height),
			GasLimit:        uint64(b.Header.GasLimit),
			MixDigest:       common.Hash{},
			Time:            b.Header.Time,
			UncleHash:       ethtypes.EmptyUncleHash,
			ReceiptHash:     ethtypes.EmptyReceiptsHash,
			BaseFee:         common.Big0,
			WithdrawalsHash: &ethtypes.EmptyWithdrawalsHash,
		}).Hash()
	}
	return b.Header.Hash
}

// This trick is played by the eth rpc server too. Instead of constructing
// an actual eth block, simply create a map with the right keys so the client
// can unmarshal it into a block
func (b *Block) ToEthLikeBlock(txs ethtypes.Transactions, inclTxs bool) map[string]any {
	excessBlobGas := hexutil.Uint64(0)
	blockGasUsed := hexutil.Uint64(0)
	result := map[string]any{
		// These are the ones that make sense to polymer.
		"parentHash": b.Header.ParentHash,
		"stateRoot":  common.BytesToHash(b.Header.AppHash),
		"number":     (*hexutil.Big)(big.NewInt(b.Header.Height)),
		"gasLimit":   hexutil.Uint64(b.Header.GasLimit),
		"mixHash":    common.Hash{},
		"timestamp":  hexutil.Uint64(b.Header.Time),
		"hash":       b.Hash(),

		// these are required fields that need to be part of the header or
		// the eth client will complain during unmarshalling
		"sha3Uncles":            ethtypes.EmptyUncleHash,
		"receiptsRoot":          ethtypes.EmptyReceiptsHash,
		"baseFeePerGas":         (*hexutil.Big)(common.Big0),
		"difficulty":            (*hexutil.Big)(common.Big0),
		"extraData":             []byte{},
		"gasUsed":               hexutil.Uint64(0),
		"logsBloom":             ethtypes.Bloom(make([]byte, ethtypes.BloomByteLength)),
		"withdrawalsRoot":       ethtypes.EmptyWithdrawalsHash,
		"withdrawals":           ethtypes.Withdrawals{},
		"blobGasUsed":           &blockGasUsed,
		"excessBlobGas":         &excessBlobGas,
		"parentBeaconBlockRoot": common.Hash{},
		"transactionsRoot":      ethtypes.DeriveSha(txs, trie.NewStackTrie(nil)),
	}
	if inclTxs {
		result["transactions"] = txs
	}
	return result
}

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
func (p *Payload) ToExecutionPayloadEnvelope(blockHash common.Hash) *opeth.ExecutionPayloadEnvelope {
	return &opeth.ExecutionPayloadEnvelope{
		ExecutionPayload: &opeth.ExecutionPayload{
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
func ValidForkchoiceUpdateResult(headBlockHash *common.Hash, id *engine.PayloadID) *opeth.ForkchoiceUpdatedResult {
	return &opeth.ForkchoiceUpdatedResult{
		PayloadStatus: opeth.PayloadStatusV1{
			Status:          opeth.ExecutionValid,
			LatestValidHash: headBlockHash,
		},
		PayloadID: id,
	}
}
