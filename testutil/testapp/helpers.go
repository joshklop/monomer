package testapp

import (
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	testappv1 "github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
	"github.com/stretchr/testify/require"
)

func NewTest(t *testing.T, chainID string) *App {
	appdb := tmdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, appdb.Close())
	})
	return New(appdb, chainID, log.NewNopLogger())
}

// ToTxs converts the key-values to SetRequest sdk.Msgs and marshals the messages to protobuf wire format.
// Each message is placed in a separate tx.
func ToTxs(t *testing.T, kvs map[string]string) [][]byte {
	var txs [][]byte
	for k, v := range kvs {
		msgAny, err := codectypes.NewAnyWithValue(&testappv1.SetRequest{
			Key:   k,
			Value: v,
		})
		require.NoError(t, err)
		tx := &sdktx.Tx{
			Body: &sdktx.TxBody{
				Messages: []*codectypes.Any{msgAny},
			},
		}
		txBytes := make([]byte, tx.Size())
		_, err = tx.MarshalTo(txBytes)
		require.NoError(t, err)
		txs = append(txs, txBytes)
	}
	return txs
}

// StateContains ensures the key-values exist in the app's state.
func (a *App) StateContains(t *testing.T, height uint64, kvs map[string]string) {
	if len(kvs) == 0 {
		return
	}
	gotState := make(map[string]string, len(kvs))
	for k := range kvs {
		requestBytes, err := (&testappv1.GetRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp := a.Query(abcitypes.RequestQuery{
			Path:   "/testapp.v1.GetService/Get", // TODO is there a way to find this programmatically?
			Data:   requestBytes,
			Height: int64(height),
		})
		var val testappv1.GetResponse
		require.NoError(t, (&val).Unmarshal(resp.GetValue()))
		gotState[k] = val.GetValue()
	}
	require.Equal(t, gotState, kvs)
}

// StateDoesNotContain ensures the key-values do not exist in the app's state.
func (a *App) StateDoesNotContain(t *testing.T, height uint64, kvs map[string]string) {
	if len(kvs) == 0 {
		return
	}
	for k := range kvs {
		requestBytes, err := (&testappv1.GetRequest{
			Key: k,
		}).Marshal()
		require.NoError(t, err)
		resp := a.Query(abcitypes.RequestQuery{
			Path:   "/testapp.v1.GetService/Get", // TODO is there a way to find this programmatically?
			Data:   requestBytes,
			Height: int64(height),
		})
		var val testappv1.GetResponse
		require.NoError(t, (&val).Unmarshal(resp.GetValue()))
		require.Empty(t, val.GetValue())
	}
}
