package genesis

import (
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	bfttypes "github.com/cometbft/cometbft/types"
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
	response := app.InitChain(abci.RequestInitChain{
		ChainId: g.ChainID.String(),
		ConsensusParams: &tmproto.ConsensusParams{
			Block: &tmproto.BlockParams{
				MaxBytes: 200000,
				MaxGas:   200000000,
			},
			Evidence: &tmproto.EvidenceParams{
				MaxAgeNumBlocks: 302400,
				MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
				MaxBytes:        10000,
			},
			Validator: &tmproto.ValidatorParams{
				PubKeyTypes: []string{
					bfttypes.ABCIPubKeyTypeEd25519,
				},
			},
		},
		AppStateBytes: g.AppState,
		Time:          time.Unix(int64(g.Time), 0),
	})

	// this will store the app state into disk. Failing to call this will result in missing data the next
	// time the app is called
	app.Commit()

	block := &eetypes.Block{
		Header: &eetypes.Header{
			Height:   0,
			ChainID:  g.ChainID,
			Time:     g.Time,
			AppHash:  response.AppHash,
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
