package config

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

func Default(cwd string) *Config {
	return &Config{
		DataDir:    cwd,
		EnginePort: 8888,
		Genesis:    &genesis.Genesis{},
	}
}

type Config struct {
	DataDir    string           `json:"data_dir"`
	EnginePort uint64           `json:"engine_port"`
	Genesis    *genesis.Genesis `json:"genesis"`
}

func runAndWrapOnError(existingErr error, msg string, fn func() error) error{
	if runErr := fn(); runErr != nil {
		if existingErr == nil {
			return runErr
		}
		runErr = fmt.Errorf("%s: %v", runErr)
		return fmt.Errorf(`failed to run because "%v" with existing err "%v"`, runErr, existingErr)
	}
	return existingErr
}

func (c *Config) Node() (*node.Node, error) {
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

func openDB(name, dir string) (tmdb.DB, error) {
	db, err := tmdb.NewDB(name, tmdb.GoLevelDBBackend, dir)
	if err != nil {
		return nil, fmt.Errorf("new db: %v", err)
	}
	return db, nil
}
