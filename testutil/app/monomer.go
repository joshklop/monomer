package app

import (
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"
)

func (a *Application) RollbackToHeight(targetHeight uint64) error {
	currentHeight := uint64(a.Info(abci.RequestInfo{}).LastBlockHeight)
	if targetHeight > currentHeight {
		return fmt.Errorf("target height %d is greater than current height %d", targetHeight, currentHeight)
	}
	for i := uint64(0); i < currentHeight-targetHeight; i++ {
		if err := a.Rollback(); err != nil {
			return fmt.Errorf("rollback to height %d: %v", currentHeight-i, err)
		}
	}
	return nil
}
