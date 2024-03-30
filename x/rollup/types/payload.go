package types

import (
	"errors"
	fmt "fmt"
	"slices"

	bfttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func AdaptPayloadTxs(ethTxs []hexutil.Bytes) (bfttypes.Txs, error) {
	if len(ethTxs) == 0 {
		return nil, errors.New("L1 Attributes transaction not found")
	}

	var numDepositTxs int
	for _, txBytes := range ethTxs {
		var tx ethtypes.Transaction
		if err := tx.UnmarshalBinary(txBytes); err != nil {
			break
		}
		numDepositTxs++
	}
	var txs [][]byte
	for _, txBytes := range ethTxs {
		txs = append(txs, txBytes)
	}

	msgAny, err := codectypes.NewAnyWithValue(&MsgL1Txs{
		TxBytes: txs,
	})
	if err != nil {
		return nil, fmt.Errorf("new any with value: %v", err)
	}
	tx := &sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: []*codectypes.Any{msgAny},
		},
	}
	txBytes := make([]byte, tx.Size())
	_, err = tx.MarshalTo(txBytes)
	if err != nil {
		return nil, fmt.Errorf("marshal tx: %v", err)
	}

	return bfttypes.ToTxs(slices.Insert(txs[numDepositTxs:], 0, txBytes)), nil
}
