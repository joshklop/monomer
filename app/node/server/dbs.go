package server

import (
	"fmt"
	"os"

	tmdb "github.com/cometbft/cometbft-db"
)

func OpenDB(name, dir string) (tmdb.DB, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir all: %v", err)
	}
	db, err := tmdb.NewDB(name, tmdb.GoLevelDBBackend, dir)
	if err != nil {
		return nil, fmt.Errorf("new db: %v", err)
	}
	return db, nil
}
