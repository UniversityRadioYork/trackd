package main

import (
	"fmt"
	"log"
	"sync"
)

type ClientChange struct {
	client *ClientHandle
	added  bool
	reply  chan<- bool
}

type ClientPool struct {
	contents map[*ClientHandle]bool
	changes  <-chan ClientChange
}

func NewClientPool() (*ClientPool, chan<- ClientChange) {
	changes := make(chan ClientChange)

	cp := new(ClientPool)
	cp.changes = changes

	return cp, changes
}

// Run Runs the client pool loop.
// It takes one argument, a waitgroup, which if non-nil will be set one higher
// during the ClientPool's lifetime.
func (cp ClientPool) Run(wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}

	for {
		select {
		case change := <-cp.changes:
			var err error = nil
			if change.added {
				err = cp.addClient(change.client)
			} else {
				err = cp.removeClient(change.client)
			}

			if err != nil {
				log.Println(err)
			}
			change.reply <- err == nil
		}
	}
}

func (cp ClientPool) addClient(client *ClientHandle) error {
	if _, ok := cp.contents[client]; ok {
		return fmt.Errorf("addClient: client %q already present", client)
	}
	cp.contents[client] = true
	return nil
}

func (cp ClientPool) removeClient(client *ClientHandle) error {
	if _, ok := cp.contents[client]; ok {
		delete(cp.contents, client)
		return nil
	}
	return fmt.Errorf("removeClient: client %q not present", client)
}
