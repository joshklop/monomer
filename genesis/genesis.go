package genesis

import (
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type Genesis struct {
	Time     uint64          `json:"genesis_time"`
	ChainID  eetypes.ChainID `json:"chain_id"`
	AppState []byte          `json:"app_state"`
}

func (g *Genesis) Commit(app peptide.Application, blockStore store.BlockStoreWriter) error {
	const initialHeight = 1
	app.InitChain(abci.RequestInitChain{
		ChainId:       g.ChainID.String(),
		AppStateBytes: g.AppState,
		Time:          time.Unix(int64(g.Time), 0),
		// The cosmos-sdk assumes the initial height is 1. If you provide 0, it will silently modify the input to 1!
		// Here is one issue: https://github.com/cosmos/cosmos-sdk/issues/16796#issue-1782358251
		// It also seems that the db version will be set to 1 even if InitialHeight is 0, which causes very confusing things later on.
		// see https://github.com/cosmos/cosmos-sdk/issues/19765
		InitialHeight: initialHeight,
	})
	response := app.Commit()

	block := &eetypes.Block{
		Header: &eetypes.Header{
			Height:   initialHeight,
			ChainID:  g.ChainID,
			Time:     g.Time,
			AppHash:  response.GetData(),
			GasLimit: peptide.DefaultGasLimit,
		},
	}
	blockStore.AddBlock(block)
	for _, label := range []eth.BlockLabel{eth.Unsafe, eth.Finalized, eth.Safe} {
		if err := blockStore.UpdateLabel(label, block.Hash()); err != nil {
			panic(err)
		}
	}
	return nil
}
