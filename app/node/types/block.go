package types

import (
	"math/big"
	"strconv"
	"time"

	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

type ChainID uint64

func (id ChainID) String() string {
	return strconv.FormatUint(uint64(id), 10)
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
		_, txCommitment := b.ethLikeTransactions()
		b.Header.Hash = (&types.Header{
			ParentHash:      b.Header.ParentHash,
			Root:            common.BytesToHash(b.Header.AppHash), // TODO actually take the keccak
			Number:          big.NewInt(b.Header.Height),
			GasLimit:        uint64(b.Header.GasLimit),
			MixDigest:       common.Hash{},
			Time:            b.Header.Time,
			TxHash:          txCommitment,
			UncleHash:       types.EmptyUncleHash,
			ReceiptHash:     types.EmptyReceiptsHash,
			BaseFee:         common.Big0,
			WithdrawalsHash: &types.EmptyWithdrawalsHash,
		}).Hash()
	}
	return b.Header.Hash
}

// This trick is played by the eth rpc server too. Instead of constructing
// an actual eth block, simply create a map with the right keys so the client
// can unmarshal it into a block
func (b *Block) ToEthLikeBlock(inclTx bool) map[string]any {
	excessBlobGas := hexutil.Uint64(0)
	blockGasUsed := hexutil.Uint64(0)

	result := map[string]any{
		// These are the ones that make sense to polymer.
		"parentHash": b.Header.ParentHash,
		"stateRoot":  common.BytesToHash(b.Header.AppHash),
		"number":     (*hexutil.Big)(big.NewInt(b.Header.Height)),
		"gasLimit":   b.Header.GasLimit,
		"mixHash":    common.Hash{},
		"timestamp":  hexutil.Uint64(b.Header.Time),
		"hash":       b.Hash(),

		// these are required fields that need to be part of the header or
		// the eth client will complain during unmarshalling
		"sha3Uncles":            types.EmptyUncleHash,
		"receiptsRoot":          types.EmptyReceiptsHash,
		"baseFeePerGas":         (*hexutil.Big)(common.Big0),
		"difficulty":            (*hexutil.Big)(common.Big0),
		"extraData":             []byte{},
		"gasUsed":               hexutil.Uint64(0),
		"logsBloom":             types.Bloom(make([]byte, types.BloomByteLength)),
		"withdrawalsRoot":       types.EmptyWithdrawalsHash,
		"withdrawals":           types.Withdrawals{},
		"blobGasUsed":           &blockGasUsed,
		"excessBlobGas":         &excessBlobGas,
		"parentBeaconBlockRoot": common.Hash{},
	}

	txs, root := b.ethLikeTransactions()
	if inclTx {
		result["transactionsRoot"] = root
		result["transactions"] = txs
	} else {
		result["transactionsRoot"] = root
	}
	return result
}

func (b *Block) ethLikeTransactions() (types.Transactions, common.Hash) {
	var txs types.Transactions
	for _, tx := range b.Txs {
		// TODO: update to use proper Gas and To values if possible
		txData := &types.DynamicFeeTx{
			ChainID: new(big.Int).SetUint64(uint64(b.Header.ChainID)),
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
