package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type ChainID struct {
	chainID *hexutil.Big
}

func NewChainID(chainID *hexutil.Big) *ChainID {
	return &ChainID{
		chainID: chainID,
	}
}

func (e *ChainID) ChainId() *hexutil.Big {
	return e.chainID
}

type BlockByNumber struct {
	blockStore store.BlockStoreReader
}

func NewBlockByNumber(blockStore store.BlockStoreReader) *BlockByNumber {
	return &BlockByNumber{
		blockStore: blockStore,
	}
}

func (e *BlockByNumber) GetBlockByNumber(id BlockID, inclTx bool) (map[string]any, error) {
	b := id.Get(e.blockStore)
	if b == nil {
		return nil, errors.New("not found")
	}
	return b.ToEthLikeBlock(inclTx), nil
}
