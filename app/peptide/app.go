package peptide

import (
	abci "github.com/cometbft/cometbft/abci/types"
)

type Application interface {
	// Info/Query Connection
	Info(abci.RequestInfo) abci.ResponseInfo    // Return application info
	Query(abci.RequestQuery) abci.ResponseQuery // Query for state

	// Mempool Connection
	CheckTx(abci.RequestCheckTx) abci.ResponseCheckTx // Validate a tx for the mempool

	// Consensus Connection
	InitChain(abci.RequestInitChain) abci.ResponseInitChain    // Initialize blockchain w validators/other info from CometBFT
	BeginBlock(abci.RequestBeginBlock) abci.ResponseBeginBlock // Signals the beginning of a block
	DeliverTx(abci.RequestDeliverTx) abci.ResponseDeliverTx    // Deliver a tx for full processing
	EndBlock(abci.RequestEndBlock) abci.ResponseEndBlock       // Signals the end of a block, returns changes to the validator set
	Commit() abci.ResponseCommit                               // Commit the state and return the application Merkle root hash

	RollbackToHeight(uint64) error
}
