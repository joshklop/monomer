package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/polymerdao/monomer/node"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		os.Exit(1)
	}
}

// run runs the Monomer node. It assumes args excludes the program name.
func run(ctx context.Context, args []string) error {
	var config *node.Config
	if numArgs := len(args); numArgs > 1 {
		return errors.New("the config file name is the only valid argument")
	} else if numArgs == 1 {
		// Parse config file.
		config = new(node.Config)
		configBytes, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("read config file: %v", err)
		}
		if err = json.Unmarshal(configBytes, &config); err != nil {
			return fmt.Errorf("unmarshal config: %v", err)
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get current working directory: %v", err)
		}
		config = node.DefaultConfig(cwd)
	}

	n, err := node.New(config)
	if err != nil {
		return fmt.Errorf("new node: %v", err)
	}

	if err := n.Start(); err != nil {
		return fmt.Errorf("start node: %v", err)
	}
	<-ctx.Done()
	if err := n.Stop(); err != nil {
		return fmt.Errorf("stop node: %v", err)
	}
	return nil
}
