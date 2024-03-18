package testapp

import (
	"testing"

	testappv1 "github.com/polymerdao/monomer/testutil/testapp/gen/testapp/v1"
	"github.com/stretchr/testify/require"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

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
