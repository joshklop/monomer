package engine

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/polymerdao/monomer/app/node/server"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type PeptideAPI struct {
	blockStore store.BlockStoreReader
	logger     server.Logger
}

func NewPeptideAPI(blockStore store.BlockStoreReader, logger server.Logger) *PeptideAPI {
	return &PeptideAPI{
		blockStore: blockStore,
		logger:     logger,
	}
}

func (e *PeptideAPI) GetBlock(id any) (*eetypes.Block, error) {
	e.logger.Debug("GetBlock", "id", id)
	telemetry.IncrCounter(1, "query", "GetBlock")

	block := e.blockByID(id)
	if block == nil {
		return nil, errors.New("block not found")
	}
	return block, nil
}

func (e *PeptideAPI) blockByID(id any) *eetypes.Block {
	switch idT := id.(type) {
	case nil:
		return e.blockStore.BlockByLabel(eth.Unsafe)
	case int64:
		return e.blockStore.BlockByNumber(idT)
	case eth.BlockLabel:
		return e.blockStore.BlockByLabel(idT)
	case []byte:
		return e.blockStore.BlockByHash(common.BytesToHash(idT))
	}
	return nil
}
