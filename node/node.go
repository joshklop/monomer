package node

import (
	"fmt"
	"math/big"
	"os"

	tmdb "github.com/cometbft/cometbft-db"
	tmlog "github.com/cometbft/cometbft/libs/log"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testutil/testapp"
)

type Config struct {
	BlockStore store.BlockStore
	TxStore    txstore.TxStore
	Mempool    *mempool.Pool // TODO interface
	Genesis    *genesis.Genesis // TODO should be network config probably.
}

type Service interface {
	Start() error
	Stop() error
}

type namedService struct {
	name    string
	service Service
}

type Node struct {
	services []namedService
}

func New(config *Config) (n *Node, err error) {
	chainID := eetypes.ChainID(1)

	logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))

	// TODO need to check if dbs are created or not.

	appdb, err := openDB("app", config.DataDir)
	if err != nil {
		return nil, err
	}
	app := testapp.New(appdb, chainID.String(), logger)

	blockdb, err := openDB("block", config.DataDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = runAndWrapOnError(err, "close block db", blockdb.Close)
	}()
	blockStore := store.NewBlockStore(blockdb)

	txdb, err := openDB("tx", config.DataDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = runAndWrapOnError(err, "close tx db", txdb.Close)
	}()
	txStore := txstore.NewTxStore(txdb)

	mempooldb, err := openDB("mempool", config.DataDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = runAndWrapOnError(err, "close mempool db", mempooldb.Close)
	}()
	mpool := mempool.New(mempooldb)

	if blockStore.HeadBlock() == nil {
		g := &genesis.Genesis{}
		if err := g.Commit(app, store.NewBlockStore(blockdb)); err != nil {
			return nil, fmt.Errorf("commit genesis: %v", err)
		}
	}

	eventBus := bfttypes.NewEventBus()
	engineAPI := engine.NewEngineAPI(builder.New(
		mpool,
		app,
		blockStore,
		txStore,
		eventBus,
		chainID,
	), blockStore)
	ethAPI := eth.NewEthAPI(
		blockStore,
		app,
		(*hexutil.Big)(new(big.Int).SetUint64(uint64(chainID))),
	)

	return &Node{
		services: []namedService{
			{
				name:    "event bus",
				service: eventBus,
			},
		},
	}, nil
}

func (n *Node) Start() error {
	for _, ns := range n.services {
		if err := ns.service.Start(); err != nil {
			return fmt.Errorf("start %s: %v", ns.name, err)
		}
	}
	return nil
}

func (n *Node) Stop() error {
	for _, ns := range n.services {
		if err := ns.service.Stop(); err != nil {
			return fmt.Errorf("stop %s: %v", ns.name, err)
		}
	}
	return nil
}

func startService(name string, s Service) error {
	if err := s.Start(); err != nil {
		return fmt.Errorf("start %s: %v", err)
	}
	return nil
}

func openDB(name, dir string) (tmdb.DB, error) {
	db, err := tmdb.NewDB(name, tmdb.GoLevelDBBackend, dir)
	if err != nil {
		return nil, fmt.Errorf("new db: %v", err)
	}
	return db, nil
}
