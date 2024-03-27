package engine

import (
	"encoding/json"
	"errors"
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	rolluptypes "github.com/joshklop/x-rollup/types"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	peptidecommon "github.com/polymerdao/monomer/app/peptide/common"
	"github.com/polymerdao/monomer/app/peptide/store"
)

type EthAPI struct {
	blockStore store.BlockStoreReader
	app        peptide.QueryableApp
	chainID    *hexutil.Big
}

func NewEthAPI(blockStore store.BlockStoreReader, app peptide.QueryableApp, chainID *hexutil.Big) *EthAPI {
	return &EthAPI{
		blockStore: blockStore,
		app:        app,
		chainID:    chainID,
	}
}

func (e *EthAPI) GetProof(address common.Address, storage []common.Hash, blockTag string) (*eth.AccountResult, error) {
	// TODO
	return nil, nil
}

func (e *EthAPI) ChainId() *hexutil.Big {
	return e.chainID
}

func (e *EthAPI) GetBalance(address common.Address, id EthBlockID) (*hexutil.Big, error) {
	b := id.Get(e.blockStore)
	if b == nil {
		return nil, errors.New("block not found")
	}

	reqBytes, err := (&banktypes.QueryBalanceRequest{
		Address: peptidecommon.EvmToCosmos(address).String(),
		Denom:   rolluptypes.ETH,
	}).Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal query request data: %v", err)
	}
	resp := e.app.Query(abci.RequestQuery{
		Data:   reqBytes,
		Path:   "/cosmos.bank.v1beta1.Query/Balance",
		Height: b.Header.Height,
	})
	if resp.IsErr() {
		return nil, fmt.Errorf("query app: %s", resp.GetLog())
	}

	respValue := new(banktypes.QueryBalanceResponse)
	if err := respValue.Unmarshal(resp.GetValue()); err != nil {
		return nil, fmt.Errorf("unmarshal response value: %v", err)
	}

	balance := respValue.GetBalance()
	if balance == nil {
		return nil, fmt.Errorf("balance not found for address %s at height %d", address, b.Header.Height)
	}

	return (*hexutil.Big)(balance.Amount.BigInt()), nil
}

func (e *EthAPI) GetBlockByHash(hash common.Hash, inclTx bool) (map[string]any, error) {
	b := e.blockStore.BlockByHash(hash)
	if b == nil {
		return nil, errors.New("not found")
	}
	return b.ToEthLikeBlock(inclTx), nil
}

type EthBlockID struct {
	label  eth.BlockLabel
	height int64
}

func (id *EthBlockID) UnmarshalJSON(data []byte) error {
	var dataStr string
	if err := json.Unmarshal(data, &dataStr); err != nil {
		return fmt.Errorf("unmarshal block id into string: %v", err)
	}

	switch dataStr {
	case eth.Unsafe, eth.Safe, eth.Finalized:
		id.label = eth.BlockLabel(dataStr)
	default:
		height := new(hexutil.Uint64)
		if err := height.UnmarshalText([]byte(dataStr)); err != nil {
			return fmt.Errorf("unmarshal height as hexutil.Uint64: %v", err)
		}
		id.height = int64(*height)
	}
	return nil
}

func (id *EthBlockID) Get(s store.BlockStoreReader) *eetypes.Block {
	if id.label != "" {
		return s.BlockByLabel(id.label)
	}
	return s.BlockByNumber(id.height)
}

func (e *EthAPI) GetBlockByNumber(id EthBlockID, inclTx bool) (map[string]any, error) {
	b := id.Get(e.blockStore)
	if b == nil {
		return nil, errors.New("not found")
	}
	return b.ToEthLikeBlock(inclTx), nil
}
