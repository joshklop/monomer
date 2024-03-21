package node

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/armon/go-metrics"
	tmdb "github.com/cometbft/cometbft-db"
	abciclient "github.com/cometbft/cometbft/abci/client"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmlog "github.com/cometbft/cometbft/libs/log"
	cmtquery "github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/libs/service"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	bfttypes "github.com/cometbft/cometbft/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	rolluptypes "github.com/joshklop/x-rollup/types"
	"github.com/polymerdao/monomer/app/node/server"
	cometbft_rpc "github.com/polymerdao/monomer/app/node/server/cometbft_rpc"
	"github.com/polymerdao/monomer/app/node/server/engine"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	peptidecommon "github.com/polymerdao/monomer/app/peptide/common"
	rpcee "github.com/polymerdao/monomer/app/peptide/rpc_ee"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/mempool"
)

const (
	AppStateDbName   = "appstate"
	BlockStoreDbName = "blockstore"
	TxStoreDbName    = "txstore"
	MempoolDbName    = "mempool"
)

type AbciClientCreator func(app abcitypes.Application) abciclient.Client

// NewLocalClient creates a new local client to ABCI server.
func NewLocalClient(app abcitypes.Application) abciclient.Client {
	return abciclient.NewLocalClient(nil, app)
}

// NewPeptideNode creates a new RPC server that
// - compatible with a subset of CometBFT's RPC API that covers txs, queries, and event subscription
// - semantically comptabile with op-geth Execution Engine API
// - supports both json RPC and websocket

func NewPeptideNode(
	bsdb tmdb.DB,
	txstoreDb tmdb.DB,
	mempooldb tmdb.DB,
	appEndpoint *server.Endpoint,
	eeEndpoint *server.Endpoint,
	chainApp peptide.Application,
	clientCreator AbciClientCreator,
	chainID eetypes.ChainID,
	logger server.Logger,
) *PeptideNode {
	bs := store.NewBlockStore(bsdb)
	txstore := txstore.NewTxStore(txstoreDb)
	node := newNode(chainApp, clientCreator, bs, txstore, mempooldb, logger.With("module", "node"))

	cometServer, cometRpcServer := cometbft_rpc.NewCometRpcServer(
		node,
		appEndpoint.FullAddress(),
		node.client,
		chainID.String(),
		logger,
	)

	config := rpcee.DefaultConfig(eeEndpoint.Host)
	eeServer := rpcee.NewEeRpcServer(config, node.getExecutionEngineAPIs(chainID, logger), logger)

	node.cometServer = cometServer
	node.cometRpcServer = cometRpcServer
	node.eeServer = eeServer
	node.nodeServices = server.NewCompositeService(node, cometRpcServer, eeServer)
	return node
}

// TODO node info should not be controlled by peptide node imho
func (p *PeptideNode) EarliestNodeInfo() cometbft_rpc.NodeInfo {
	return cometbft_rpc.NodeInfo{
		BlockHash:   []byte{},
		AppHash:     []byte{},
		BlockHeight: 0,
		Time:        time.Now(),
	}
}

func (p *PeptideNode) ReportMetrics() {}

func (p *PeptideNode) LastNodeInfo() cometbft_rpc.NodeInfo {
	return p.EarliestNodeInfo()
}

// The public rpc methods are prefixed by the namespace (lower case) followed by all exported
// methods of the "service" in camelcase
func (p *PeptideNode) getExecutionEngineAPIs(chainID eetypes.ChainID, logger server.Logger) []ethrpc.API {
	return []ethrpc.API{
		{
			Namespace: "engine",
			Service: engine.NewEngineAPI(
				builder.New(p.txMempool, p.app, p.bs, p.txstore, p.eventBus, chainID),
				p.bs,
			),
		}, {
			Namespace: "eth",
			Service:   engine.NewEthAPI(p.bs, p, (*hexutil.Big)(new(big.Int).SetUint64(uint64(chainID)))),
		}, {
			Namespace: "pep",
			Service:   engine.NewPeptideAPI(p.bs, logger.With("module", "peptide")),
		},
	}
}

// PeptideNode implements all RPC methods defined in RouteMap.
type PeptideNode struct {
	client        abciclient.Client
	clientCreator AbciClientCreator
	app           peptide.Application
	lock          sync.RWMutex

	eventBus *bfttypes.EventBus
	txstore  txstore.TxStore
	logger   tmlog.Logger

	bs store.BlockStore

	// L2 txs are stored in mempool until block is sealed
	txMempool *mempool.Pool

	// Node components
	cometServer    *cometbft_rpc.CometServer
	cometRpcServer *cometbft_rpc.RPCServer
	eeServer       *rpcee.EERPCServer
	*service.BaseService
	nodeServices service.Service
}

// Service returns the composite service that implements
// - node services: p2p, event bus, etc.
// - CometBFT's RPC API
// - Execution Engine API.
//
// Must use this method to start/stop the node.
func (cs *PeptideNode) Service() service.Service {
	return cs.nodeServices
}

// CometServerAddress returns the address of the cometbft rpc server.
func (cs *PeptideNode) CometServerAddress() net.Addr {
	return cs.cometRpcServer.Address()
}

// EngineServerAddress returns the address of the execution engine rpc server.
func (cs *PeptideNode) EngineServerAddress() net.Addr {
	return cs.eeServer.Address()
}

// TODO: do not expose PeptideNode as a service to avoid misuse
var _ service.Service = (*PeptideNode)(nil)

func newNode(chainApp peptide.Application, clientCreator AbciClientCreator, bs store.BlockStore,
	txstore txstore.TxStore, mempoolStorage tmdb.DB, logger tmlog.Logger,
) *PeptideNode {
	cs := &PeptideNode{
		clientCreator: clientCreator,
		app:           chainApp,
		logger:        logger,
		bs:            bs,
		txstore:       txstore,
		lock:          sync.RWMutex{},
		txMempool:     mempool.New(mempoolStorage),
	}
	cs.BaseService = service.NewBaseService(logger, "PeptideNode", cs)

	cs.resume()
	cs.resetClient()
	return cs
}

func (cs *PeptideNode) resume() {
	lastBlock := cs.bs.BlockByLabel(eth.Unsafe)
	if lastBlock == nil {
		panic("could not load current block")
	}

	// in the odd case the app state comes up out of sync with the blockstore, we perform a mini-rollback
	// to bring them back to the same place. This should never ever happen but when it does (and it did)
	// it would cause the loop derivation to get stuck
	info := cs.app.Info(abcitypes.RequestInfo{})
	if lastBlock.Header.Height != info.LastBlockHeight {
		cs.logger.Info("blockstore and appstate out of sync",
			"last_block_height",
			lastBlock.Header.Height,
			"app_last_height",
			info.LastBlockHeight)

		// because the appstate is *always* comitted before the blockstore, the only scenario where there'd be
		// a mismatch is if the appstate is ahead by 1. Other situation would mean something else is broken
		// and there's no point in trying to fix it at runtime.
		if lastBlock.Header.Height+1 != info.LastBlockHeight {
			panic("difference between blockstore and appstate is higher than 1")
		}

		// do the mini-rollback to the last height available on the block store
		if err := cs.app.RollbackToHeight(uint64(lastBlock.Header.Height)); err != nil {
			panic(err)
		}
	}
}

// OnStart starts the chain server.
func (cs *PeptideNode) OnStart() error {
	// create and start event bus
	cs.eventBus = bfttypes.NewEventBus()
	cs.eventBus.SetLogger(cs.logger.With("module", "events"))
	if err := cs.eventBus.Start(); err != nil {
		cs.logger.Error("failed to start event bus", "err", err)
		log.Fatalf("failed to start event bus: %s", err)
	}

	return nil
}

// OnStop cleans up resources of clientRoute.
func (cs *PeptideNode) OnStop() {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	if cs.eventBus != nil {
		cs.eventBus.Stop()
	}
	metrics.Shutdown()
}

// OnWebsocketDisconnect is called when a websocket connection is disconnected.
func (cs *PeptideNode) OnWebsocketDisconnect(remoteAddr string, logger tmlog.Logger) {
	logger.Info("OnDisconnect", "remote", remoteAddr)
	err := cs.eventBus.UnsubscribeAll(context.Background(), remoteAddr)
	if err != nil {
		logger.Error("failed to unsubscribe events", "remote", remoteAddr, "err", err)
	}
}

// AddToTxMempool adds txs to the mempool.
func (cs *PeptideNode) AddToTxMempool(tx bfttypes.Tx) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if err := cs.txMempool.Enqueue([]byte(tx)); err != nil {
		panic(fmt.Errorf("enqueue: %v", err))
	}
}

type ValidatorInfo = ctypes.ValidatorInfo

func (cs *PeptideNode) ValidatorInfo() ValidatorInfo {
	return ctypes.ValidatorInfo{}
}

func (cs *PeptideNode) EventBus() *bfttypes.EventBus {
	return cs.eventBus
}

func (cs *PeptideNode) GetTxByHash(hash []byte) (*abcitypes.TxResult, error) {
	return cs.txstore.Get(hash)
}

func (cs *PeptideNode) SearchTx(ctx context.Context, q *cmtquery.Query) ([]*abcitypes.TxResult, error) {
	return cs.txstore.Search(ctx, q)
}

func (cs *PeptideNode) getBlockByNumber(number int64) *eetypes.Block {
	switch ethrpc.BlockNumber(number) {
	// optimism expects these two to be the same
	case ethrpc.PendingBlockNumber, ethrpc.LatestBlockNumber:
		return cs.bs.BlockByLabel(eth.Unsafe)
	case ethrpc.SafeBlockNumber:
		return cs.bs.BlockByLabel(eth.Safe)
	case ethrpc.FinalizedBlockNumber:
		return cs.bs.BlockByLabel(eth.Finalized)
	case ethrpc.EarliestBlockNumber:
		return cs.bs.BlockByNumber(0) // TODO is genesis block number always zero?
	default:
		return cs.bs.BlockByNumber(number)
	}
}

func (cs *PeptideNode) getBlockByString(str string) *eetypes.Block {
	// use base 0 so it's autodetected
	number, err := strconv.ParseInt(str, 0, 64)
	if err == nil {
		return cs.getBlockByNumber(number)
	}
	// When block number is ethrpc.PendingBlockNumber, optimsim expects the latest block.
	// See https://github.com/ethereum-optimism/optimism/blob/v1.2.0/op-e2e/system_test.go#L1353
	// The ethclient converts negative int64 numbers to their respective labels and that's what
	// the server (us) gets. i.e. ethrpc.PendingBlockNumber (-1) is converted to "pending"
	// See https://github.com/ethereum-optimism/op-geth/blob/v1.101304.1/rpc/types.go
	// Since "pending" is no a label we use elsewhere, we need to check for it here
	// and returna the latest (unsafe) block
	if str == "pending" {
		return cs.bs.BlockByLabel(eth.Unsafe)
	}
	return cs.bs.BlockByLabel(eth.BlockLabel(str))
}

func (cs *PeptideNode) GetBlock(id any) (*eetypes.Block, error) {
	cs.logger.Info("trying: PeptideNode.GetBlock", "id", id)
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	cs.logger.Info("PeptideNode.GetBlock", "id", id)

	block, err := func() (*eetypes.Block, error) {
		switch v := id.(type) {
		case nil:
			return cs.bs.BlockByLabel(eth.Unsafe), nil
		case []byte:
			return cs.bs.BlockByHash(common.Hash(v)), nil
		case int64:
			return cs.getBlockByNumber(v), nil
		// sometimes int values are weirdly converted to float?
		case float64:
			return cs.getBlockByNumber(int64(v)), nil
		case string:
			return cs.getBlockByString(v), nil
		default:
			return nil, fmt.Errorf("cannot query block by value %v (%T)", v, id)
		}
	}()
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, ethereum.NotFound
	}
	return block, nil
}

// TODO: add more details to response
type ImportExportResponse struct {
	success bool
	path    string
	height  int64
}

// resetClient creates a new client and stops the old one.
func (cs *PeptideNode) resetClient() {
	if cs.client != nil && cs.client.IsRunning() {
		cs.client.Stop()
	}
	cs.client = cs.clientCreator(cs.app)

	// TODO: allow to enable/disable tx indexer; and add logger when cometbft version is upgraded
}

// GetETH returns the wrapped ETH balance in Wei of the given EVM address.
func (cs *PeptideNode) Balance(evmAddr common.Address, height int64) (*big.Int, error) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	cosmAddr := peptidecommon.EvmToCosmos(evmAddr)

	resp := peptide.MustGetResponseWithHeight(new(banktypes.QueryBalanceResponse), cs.app, &banktypes.QueryBalanceRequest{
		Address: cosmAddr.String(),
		Denom:   rolluptypes.ETH,
	}, "/cosmos.bank.v1beta1.Query/Balance", height)
	balance := resp.Balance.Amount.BigInt()

	return balance, nil
}
