package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	//"path/filepath"
	"syscall"
	//"time"

	//abciclient "github.com/cometbft/cometbft/abci/client"
	//abcitypes "github.com/cometbft/cometbft/abci/types"
	//"github.com/ethereum-optimism/optimism/op-service/eth"
	//"github.com/polymerdao/monomer/app/node"
	//"github.com/polymerdao/monomer/app/node/server"
	//eetypes "github.com/polymerdao/monomer/app/node/types"
	//"github.com/polymerdao/monomer/app/peptide"
	//"github.com/polymerdao/monomer/app/peptide/store"
	//"github.com/polymerdao/monomer/testutil/testapp"
)

type config struct {
	DataDir    string
	CometPort  uint64
	EnginePort uint64
}

func main() {
	cfg := new(config)
	flag.StringVar(&cfg.DataDir, "data-dir", "", "data directory.")
	flag.Uint64Var(&cfg.CometPort, "comet-port", 6060, "comet port.")
	flag.Uint64Var(&cfg.EnginePort, "engine-port", 6061, "engine port.")
	flag.Parse()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-quit
		cancel()
	}()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config) error {
	return nil
}
	/*
	app, err := testapp.New()
	if err != nil {
		return fmt.Errorf("create new application: %v", err)
	}

	blockdb, err := server.OpenDB("block", cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open block db: %v", err)
	}
	defer blockdb.Close() // TODO check error

	txdb, err := server.OpenDB("tx", cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open tx db: %v", err)
	}
	defer txdb.Close() // TODO check error

	mempooldb, err := server.OpenDB("mempool", cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open mempool db: %v", err)
	}
	defer mempooldb.Close() // TODO check error

	chainID := eetypes.ChainID(1)
	if _, err = node.InitChain(app, store.NewBlockStore(blockdb), &node.PeptideGenesis{
		GenesisTime: time.Now(),
		ChainID:     chainID.String(),
		AppState: []byte(`{
				"key": "value"
			}`),
		L1: eth.BlockID{
			Number: 1,
		},
		InitialL2Height: 1,
	}); err != nil {
		return fmt.Errorf("init chain: %v", err)
	}

	peptideNode := node.NewPeptideNode(
		blockdb,
		txdb,
		mempooldb,
		&server.Endpoint{
			Host:     fmt.Sprintf("localhost:%d", cfg.CometPort),
			Protocol: "tcp",
		},
		&server.Endpoint{
			Host:     fmt.Sprintf("localhost:%d", cfg.EnginePort),
			Protocol: "tcp",
		},
		peptide.New(chainID, app),
		func(app abcitypes.Application) abciclient.Client {
			return nil // TODO
		},
		server.DefaultLogger(),
	)

	if err := peptideNode.Service().Start(); err != nil {
		return fmt.Errorf("node: %v", err)
	}
	<-ctx.Done()
	return nil
}
*/
