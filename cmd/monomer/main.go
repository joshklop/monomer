package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"

	tmdb "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmlog "github.com/cometbft/cometbft/libs/log"
	bfttypes "github.com/cometbft/cometbft/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/app/peptide"
	rpcee "github.com/polymerdao/monomer/app/peptide/rpc_ee"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/polymerdao/monomer/builder"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/mempool"
	"github.com/polymerdao/monomer/testutil/testapp"
)

type Config struct {
	DataDir    string           `json:"data_dir"`
	EngineHost string           `json:"engine_host"`
	EnginePort uint16           `json:"engine_port"`
	Genesis    *genesis.Genesis `json:"genesis"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current working directory: %v", err)
	}

	var dataDir string
	flag.StringVar(&dataDir, "data-dir", cwd, "")
	var engineHost string
	flag.StringVar(&engineHost, "engine-host", "127.0.0.1", "")
	var enginePort uint64
	flag.Uint64Var(&enginePort, "engine-port", 8888, "")
	var genesisFile string
	flag.StringVar(&genesisFile, "genesis-file", "", "")

	flag.Parse()

	if enginePort > math.MaxUint16 {
		return nil, fmt.Errorf("engine port is out of range: %d", enginePort)
	}

	g := new(genesis.Genesis)
	if genesisFile != "" {
		genesisBytes, err := os.ReadFile(genesisFile)
		if err != nil {
			return nil, fmt.Errorf("read genesis file: %v", err)
		}

		if err = json.Unmarshal(genesisBytes, &g); err != nil {
			return nil, fmt.Errorf("unmarshal genesis file: %v", err)
		}
	}

	return &Config{
		DataDir:    dataDir,
		EngineHost: engineHost,
		EnginePort: uint16(enginePort),
		Genesis:    g,
	}, nil
}

// run runs the Monomer node. It assumes args excludes the program name.
func run(ctx context.Context) error {
	config, err := parseFlags()
	if err != nil {
		return err
	}

	logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))

	appdb, err := openDB("app", config.DataDir)
	if err != nil {
		return err
	}
	app := testapp.New(appdb, config.Genesis.ChainID.String(), logger)

	blockdb, err := openDB("block", config.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		err = runAndWrapOnError(err, "close block db", blockdb.Close)
	}()
	blockStore := store.NewBlockStore(blockdb)

	if err := prepareBlockStoreAndApp(config.Genesis, blockStore, app); err != nil {
		return err
	}

	txdb, err := openDB("tx", config.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		err = runAndWrapOnError(err, "close tx db", txdb.Close)
	}()
	txStore := txstore.NewTxStore(txdb)

	mempooldb, err := openDB("mempool", config.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		err = runAndWrapOnError(err, "close mempool db", mempooldb.Close)
	}()
	mpool := mempool.New(mempooldb)

	eventBus := bfttypes.NewEventBus()
	if err != nil {
		return fmt.Errorf("listen on engine address: %v", err)
	}

	n := newNodeService(rpcee.NewEeRpcServer(config.EngineHost, config.EnginePort, []ethrpc.API{
		{
			Namespace: "eth",
			Service:   engine.NewEthAPI(blockStore, app, config.Genesis.ChainID.HexBig()),
		},
		{
			Namespace: "engine",
			Service:   engine.NewEngineAPI(builder.New(mpool, app, blockStore, txStore, eventBus, config.Genesis.ChainID), blockStore),
		},
	}, logger), eventBus)

	if err := n.Start(); err != nil {
		return fmt.Errorf("start node: %v", err)
	}
	<-ctx.Done()
	if err := n.Stop(); err != nil {
		return fmt.Errorf("stop node: %v", err)
	}
	return nil
}

func prepareBlockStoreAndApp(g *genesis.Genesis, blockStore store.BlockStore, app peptide.Application) error {
	// Get blockStoreHeight and appHeight.
	var blockStoreHeight uint64
	if headBlock := blockStore.HeadBlock(); headBlock != nil {
		blockStoreHeight = uint64(headBlock.Header.Height)
	}
	info := app.Info(abcitypes.RequestInfo{})
	appHeight := uint64(info.GetLastBlockHeight())

	// Ensure appHeight == blockStoreHeight.
	if appHeight == blockStoreHeight+1 {
		// There is a possibility that we committed to the app and Monomer crashed before committing to the block store.
		if err := app.RollbackToHeight(blockStoreHeight); err != nil {
			return fmt.Errorf("rollback app: %v", err)
		}
	} else if appHeight > blockStoreHeight {
		return fmt.Errorf("app height %d is too far ahead of block store height %d", appHeight, blockStoreHeight)
	} else if appHeight < blockStoreHeight {
		return fmt.Errorf("app height %d is behind block store height %d", appHeight, blockStoreHeight)
	}

	// Commit genesis.
	if blockStoreHeight == 0 { // We know appHeight == blockStoreHeight at this point.
		if err := g.Commit(app, blockStore); err != nil {
			return fmt.Errorf("commit genesis: %v", err)
		}
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

func runAndWrapOnError(existingErr error, msg string, fn func() error) error {
	if runErr := fn(); runErr != nil {
		if existingErr == nil {
			return runErr
		}
		runErr = fmt.Errorf("%s: %v", msg, runErr)
		return fmt.Errorf(`failed to run because "%v" with existing err "%v"`, runErr, existingErr)
	}
	return existingErr
}
