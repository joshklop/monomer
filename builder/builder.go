package builder

import (
	"fmt"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/mempool"
)

type Builder struct {
	mempool    *mempool.Pool
	app        peptide.Application
	blockStore store.BlockStore
	txStore    txstore.TxStore
	eventBus   *bfttypes.EventBus
	chainID    eetypes.ChainID
}

func New(
	mempool *mempool.Pool,
	app peptide.Application,
	blockStore store.BlockStore,
	txStore txstore.TxStore,
	eventBus *bfttypes.EventBus,
	chainID eetypes.ChainID,
) *Builder {
	return &Builder{
		mempool:    mempool,
		app:        app,
		blockStore: blockStore,
		txStore:    txStore,
		eventBus:   eventBus,
		chainID:    chainID,
	}
}

// safe and finalized may be common.Hash{}, in which case they are not updated.
// it is assumed that all non-empty hashes exist in the block store.
func (b *Builder) Rollback(head, safe, finalized common.Hash) error {
	header := b.blockStore.BlockByHash(head)
	if header == nil {
		return fmt.Errorf("block not found with hash %s", head)
	}
	height := header.Header.Height

	b.blockStore.RollbackToHeight(height)
	b.blockStore.UpdateLabel(eth.Unsafe, head)
	if safe != (common.Hash{}) {
		b.blockStore.UpdateLabel(eth.Safe, safe)
	}
	if finalized != (common.Hash{}) {
		b.blockStore.UpdateLabel(eth.Finalized, finalized)
	}

	b.txStore.RollbackToHeight(height, b.blockStore.HeadBlock().Header.Height)

	if err := b.app.RollbackToHeight(uint64(height)); err != nil {
		return fmt.Errorf("rollback app: %v", err)
	}

	return nil
}

type Payload struct {
	// Transactions functions as an inclusion list.
	Transactions bfttypes.Txs
	GasLimit     uint64
	Timestamp    uint64
}

func (b *Builder) Build(payload *Payload) error {
	var txs bfttypes.Txs
	copy(txs, payload.Transactions)
	for {
		// TODO there is risk of losing txs if mempool db fails.
		// we need to fix db consistency in general, so we're just panicing on errors for now.
		length, err := b.mempool.Len()
		if err != nil {
			panic(fmt.Errorf("enqueue: %v", err))
		}
		if length == 0 {
			break
		}

		tx, err := b.mempool.Dequeue()
		if err != nil {
			panic(fmt.Errorf("dequeue: %v", err))
		}
		txs = append(txs, tx)
	}

	// Build header.
	info := b.app.Info(abcitypes.RequestInfo{})
	currentHead := b.blockStore.BlockByNumber(info.GetLastBlockHeight())
	if currentHead == nil {
		return fmt.Errorf("block not found at height: %d", info.GetLastBlockHeight())
	}
	header := &eetypes.Header{
		ChainID:    b.chainID,
		Height:     currentHead.Header.Height + 1,
		Time:       uint64(payload.Timestamp),
		ParentHash: currentHead.Header.Hash,
		AppHash:    info.GetLastBlockAppHash(),
		GasLimit:   uint64(payload.GasLimit),
	}

	// BeginBlock, DeliverTx, EndBlock, Commit
	b.app.BeginBlock(abcitypes.RequestBeginBlock{
		Header: *header.ToComet().ToProto(),
	})
	var txResults []*abcitypes.TxResult
	for i, tx := range txs {
		resp := b.app.DeliverTx(abcitypes.RequestDeliverTx{
			Tx: tx,
		})
		txResults = append(txResults, &abcitypes.TxResult{
			Height: info.GetLastBlockHeight() + 1,
			Tx:     tx,
			Index:  uint32(i),
			Result: resp,
		})
	}
	b.app.EndBlock(abcitypes.RequestEndBlock{
		Height: info.GetLastBlockHeight() + 1,
	})
	b.app.Commit()

	// Append block.
	block := &eetypes.Block{
		Header:    header,
		Txs:       txs,
		TxResults: txResults,
	}
	block.Hash()
	b.blockStore.AddBlock(block)
	// Index txs.
	if err := b.txStore.Add(block.TxResults); err != nil {
		return fmt.Errorf("add tx results: %v", err)
	}
	// Publish events.
	for _, txResult := range block.TxResults {
		if err := b.eventBus.PublishEventTx(bfttypes.EventDataTx{
			TxResult: *txResult,
		}); err != nil {
			return fmt.Errorf("publish event tx: %v", err)
		}
	}
	return nil
}
