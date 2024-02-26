package engine

import (
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
)

type PeptideAPI struct {
	node   Node
	logger server.Logger
}

func NewPeptideAPI(node Node, logger server.Logger) *PeptideAPI {
	return &PeptideAPI{
		node:   node,
		logger: logger,
	}
}

func (e *PeptideAPI) GetBlock(id any) (*eetypes.Block, error) {
	e.logger.Debug("GetBlock", "id", id)
	telemetry.IncrCounter(1, "query", "GetBlock")

	block, err := e.node.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return block, nil
}
