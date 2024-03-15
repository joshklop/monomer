package app_test

import (
	"testing"

	keepertest "app/testutil/keeper"
	"app/testutil/nullify"
	"app/x/app"
	"app/x/app/types"
	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params: types.DefaultParams(),

		// this line is used by starport scaffolding # genesis/test/state
	}

	k, ctx := keepertest.AppKeeper(t)
	app.InitGenesis(ctx, *k, genesisState)
	got := app.ExportGenesis(ctx, *k)
	require.NotNil(t, got)

	nullify.Fill(&genesisState)
	nullify.Fill(got)

	// this line is used by starport scaffolding # genesis/test/assert
}
