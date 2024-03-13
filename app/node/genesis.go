package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
)

type PeptideGenesis struct {
	GenesisTime time.Time   `json:"genesis_time"`
	ChainID     string      `json:"chain_id"`
	AppState    []byte      `json:"app_state"`
	L1          eth.BlockID `json:"l1"`
	// InitialL2Height is usually 0 (Comet uses 1, we use 0).
	// It may be greater than zero in the event of chain restarts.
	// https://docs.cometbft.com/v0.38/spec/core/genesis
	InitialL2Height int64 `json:"initial_height"`
}

func (p *PeptideGenesis) Validate() error {
	if p.L1.Hash.Cmp(common.Hash{}) == 0 {
		return fmt.Errorf("l1 hash must not be empty")
	}
	if p.GenesisTime.Unix() == int64(0) {
		return fmt.Errorf("genesis time must not be empty")
	}
	if _, err := strconv.ParseUint(p.ChainID, 10, 64); err != nil {
		return fmt.Errorf("chain-id must be numerical")
	}
	if len(p.AppState) == 0 {
		return fmt.Errorf("app state must not be empty")
	}
	return nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (p *PeptideGenesis) Save(homedir string, override bool) error {
	genFile := filepath.Join(homedir, "config", "genesis.json")
	if err := os.MkdirAll(filepath.Dir(genFile), 0o700); err != nil {
		return err
	}

	// return error if the genesis file exist already
	if !override && fileExists(genFile) {
		return fmt.Errorf("refusing to override existent genesis file %v", genFile)
	}

	genesis, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(genFile, genesis, 0o644); err != nil {
		return err
	}

	return nil
}

func PeptideGenesisFromFile(homedir string) (*PeptideGenesis, error) {
	genFile := filepath.Join(homedir, "config", "genesis.json")

	bz, err := os.ReadFile(genFile)
	if err != nil {
		return nil, fmt.Errorf("could not read genesis file: %w", err)
	}

	var genesis PeptideGenesis
	if err := json.Unmarshal(bz, &genesis); err != nil {
		return nil, fmt.Errorf("could not unmarshal genesis file: %w", err)
	}

	if err := genesis.Validate(); err != nil {
		return nil, err
	}

	return &genesis, nil
}
