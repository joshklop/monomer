package main

import (
	"fmt"
)

type Service interface {
	Start() error
	Stop() error
}

type namedService struct {
	name    string
	service Service
}

type nodeService []namedService

func newNodeService(engineServer, eventBus Service) nodeService {
	return []namedService{
		{
			name:    "engine server",
			service: engineServer,
		},
		{
			name:    "event bus",
			service: eventBus,
		},
	}
}

func (n nodeService) Start() error {
	for _, namedService := range n {
		if err := namedService.service.Start(); err != nil {
			return fmt.Errorf("start %s: %v", namedService.name, err)
		}
	}
	return nil
}

func (n nodeService) Stop() error {
	for _, namedService := range n {
		if err := namedService.service.Stop(); err != nil {
			return fmt.Errorf("stop %s: %v", namedService.name, err)
		}
	}
	return nil
}
