package types

import (
	"fmt"
	"math/big"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

type Header struct {
	ChainID string `json:"chain_id"`
	Height  int64  `json:"height"`
	Time    uint64 `json:"time"`

	// prev block hash
	LastBlockHash []byte `json:"last_block_hash"`

	// hashes of block data
	LastCommitHash []byte `json:"last_commit_hash"` // commit from validators from the last block
	DataHash       []byte `json:"data_hash"`        // transactions

	// hashes from the app output from the prev block
	ValidatorsHash     []byte `json:"validators_hash"`      // validators for the current block
	NextValidatorsHash []byte `json:"next_validators_hash"` // validators for the next block
	ConsensusHash      []byte `json:"consensus_hash"`       // consensus params for current block
	AppHash            []byte `json:"app_hash"`             // state after txs from the previous block
	// root hash of all results from the txs from the previous block
	LastResultsHash []byte `json:"last_results_hash"`

	// consensus info
	EvidenceHash []byte `json:"evidence_hash"` // evidence included in the block
}

func (h *Header) Populate(cosmosHeader *tmproto.Header) *Header {
	h.ChainID = cosmosHeader.ChainID
	h.Height = cosmosHeader.Height
	h.Time = uint64(cosmosHeader.Time.Unix())
	h.LastBlockHash = cosmosHeader.LastBlockId.Hash
	h.LastCommitHash = cosmosHeader.LastCommitHash
	h.DataHash = cosmosHeader.DataHash
	h.ValidatorsHash = cosmosHeader.ValidatorsHash
	h.NextValidatorsHash = cosmosHeader.NextValidatorsHash
	h.ConsensusHash = cosmosHeader.ConsensusHash
	h.AppHash = cosmosHeader.AppHash
	h.LastResultsHash = cosmosHeader.LastResultsHash
	h.EvidenceHash = cosmosHeader.EvidenceHash
	return h
}

type Block struct {
	Txs             bfttypes.Txs       `json:"txs"`
	Header          *Header            `json:"header"`
	ParentBlockHash common.Hash        `json:"parentHash"`
	GasLimit        hexutil.Uint64     `json:"gasLimit"`
	BlockHash       common.Hash        `json:"hash"`
	PrevRandao      eth.Bytes32        `json:"prevRandao"`
	Withdrawals     *types.Withdrawals `json:"withdrawals,omitempty"`
}

func (b *Block) Height() int64 {
	return b.Header.Height
}

// Hash returns a unique hash of the block, used as the block identifier
func (b *Block) Hash() common.Hash {
	if b.BlockHash == (common.Hash{}) {
		header := types.Header{}
		header.ParentHash = b.ParentHash()
		header.Root = common.BytesToHash(b.Header.AppHash)
		header.Number = big.NewInt(b.Height())
		header.GasLimit = uint64(b.GasLimit)
		header.MixDigest = common.Hash(b.PrevRandao)
		header.Time = b.Header.Time
		_, header.TxHash = b.Transactions()

		// these are set to "empty" stuff but they are needed to corre
		// a correct
		header.UncleHash = types.EmptyUncleHash
		header.ReceiptHash = types.EmptyReceiptsHash
		header.BaseFee = common.Big0
		header.WithdrawalsHash = &types.EmptyWithdrawalsHash

		hash := header.Hash()
		copy(b.BlockHash[:], hash[:])
	}
	return b.BlockHash
}

func (b *Block) ParentHash() common.Hash {
	return b.ParentBlockHash
}

func (b *Block) Transactions() (types.Transactions, common.Hash) {
	chainId, ok := big.NewInt(0).SetString(b.Header.ChainID, 10)
	if !ok {
		panic(fmt.Sprintf("block chain id is not an integer %s", b.Header.ChainID))
	}

	var txs types.Transactions
	for _, tx := range b.Txs {
		// TODO: update to use proper Gas and To values if possible
		txData := &types.DynamicFeeTx{
			ChainID: chainId,
			Data:    tx,
			Gas:     0,
			Value:   big.NewInt(0),
			To:      nil,
		}
		tx := types.NewTx(txData)
		txs = append(txs, tx)
	}
	return txs, types.DeriveSha(txs, trie.NewStackTrie(nil))
}
