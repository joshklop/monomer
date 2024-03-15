package keeper

import (
	"app/x/app/types"
)

var _ types.QueryServer = Keeper{}
