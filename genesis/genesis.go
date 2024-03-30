package genesis

import (
	"encoding/json"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type Genesis struct {
	Time     uint64          `json:"time"`
	ChainID  eetypes.ChainID `json:"chain_id"`
	AppState []byte          `json:"app_state"`
}

const defaultGasLimit = 30_000_000

// Commit assumes the application has not been initialized and that the block store is empty.
func (g *Genesis) Commit(app peptide.Application, blockStore store.BlockStoreWriter) error {
	// Sanity check the json.
	{
		b := new(json.RawMessage)
		if err := json.Unmarshal(g.AppState, &b); err != nil {
			return fmt.Errorf("app state is invalid json: %v", err)
		}
	}

	const initialHeight = 1
	app.InitChain(abci.RequestInitChain{
		ChainId:       g.ChainID.String(),
		AppStateBytes: g.AppState,
		Time:          time.Unix(int64(g.Time), 0),
		// If the initial height is not set, the cosmos-sdk will silently set it to 1.
		// https://github.com/cosmos/cosmos-sdk/issues/19765
		InitialHeight: initialHeight,
	})
	response := app.Commit()

	block := &eetypes.Block{
		Header: &eetypes.Header{
			Height:   initialHeight,
			ChainID:  g.ChainID,
			Time:     g.Time,
			AppHash:  response.GetData(),
			GasLimit: defaultGasLimit,
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
