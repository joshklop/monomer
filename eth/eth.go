package eth

import (
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

func (e *EthAPI) GetBalance(address common.Address, id any) (*hexutil.Big, error) {
	b := e.blockByID(id)
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
		return nil, errors.New("block not found")
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) GetBlockByNumber(id any, inclTx bool) (map[string]any, error) {
	b := e.blockByID(id)
	if b == nil {
		return nil, errors.New("block not found") // TODO: do we need a special error?
	}
	return b.ToEthLikeBlock(inclTx), nil
}

func (e *EthAPI) blockByID(id any) *eetypes.Block {
	switch idT := id.(type) {
	case nil:
		return e.blockStore.BlockByLabel(eth.Unsafe)
	case int:
		return e.blockStore.BlockByNumber(int64(idT))
	case int64:
		return e.blockStore.BlockByNumber(idT)
	case eth.BlockLabel:
		return e.blockStore.BlockByLabel(idT)
	case string:
		return e.blockStore.BlockByLabel(eth.BlockLabel(idT))
	default:
		return nil
	}
}
