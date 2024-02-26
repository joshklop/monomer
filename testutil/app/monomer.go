package app

import (
	"fmt"
)

func (a *Application) RollbackToHeight(targetHeight uint64) error {
	currentHeight := a.state.Height
	if targetHeight > currentHeight {
		return fmt.Errorf("target height %d is greater than current height %d", targetHeight, currentHeight)
	}
	for i := uint64(0); i < currentHeight-targetHeight; i++ {
		if err := a.state.Rollback(); err != nil {
			return fmt.Errorf("rollback to height %d: %v", currentHeight-i+1, err)
		}
	}
	return nil
}
