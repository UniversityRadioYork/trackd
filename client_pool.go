package main

import (
	"fmt"
	"log"

	"github.com/UniversityRadioYork/baps3-go"
	"gopkg.in/tomb.v2"
)

type ClientHandle struct {
	// Channel for sending broadcast messages to this client.
	Broadcast chan<- *baps3.Message
	// Channel for sending disconnect messages to this client.
	Disconnect chan<- struct{}
}

// ClientChange is a request to the client pool to add or remove a client.
type ClientChange struct {
	client *ClientHandle
	added  bool
	reply  chan<- bool
}

type ClientPool struct {
	contents  map[*ClientHandle]struct{}
	changes   <-chan ClientChange
	quit      <-chan struct{}
	broadcast <-chan *baps3.Message
	quitting  bool
}

// ClientPoolHandle contains a ClientPool and channels to communicate with it
// while it is running.
type ClientPoolHandle struct {
	Pool      *ClientPool
	Changes   chan<- ClientChange
	Broadcast chan<- *baps3.Message
}

// NewClientPool creates a new client pool.
// It returns a ClientPoolHandle.
func NewClientPool(quit chan struct{}) ClientPoolHandle {
	changes := make(chan ClientChange)
	broadcast := make(chan *baps3.Message)

	cp := ClientPool{
		contents:  make(map[*ClientHandle]struct{}),
		changes:   changes,
		quit:      quit,
		broadcast: broadcast,
		quitting:  false,
	}

	return ClientPoolHandle{
		Pool:      &cp,
		Changes:   changes,
		Broadcast: broadcast,
	}
}

// Run runs the client pool loop.
// It takes one argument:
//   t: a Tomb, whose death causes this client pool to die.
func (cp ClientPool) Run(t *tomb.Tomb) (err error) {
	defer func() { log.Println("client pool is closing") }()

	for {
		select {
		case change := <-cp.changes:
			cp.handleClientChange(change)

			log.Printf("clientPool: now %d clients", len(cp.contents))

			// If we're quitting, we're now waiting for all of the
			// connections to close so we can quit.
			if cp.quitting && 0 == len(cp.contents) {
				return
			}
		case broadcast := <-cp.broadcast:
			log.Println("broadcast: %q", broadcast)
			for client, _ := range cp.contents {
				client.Broadcast <- broadcast
			}
		case <-t.Dying():
			log.Println("client pool is beginning to close")

			cp.quitting = true

			// If we don't have any connections, then close right
			// now.  Otherwise, we wait for those connections to
			// close.
			if 0 == len(cp.contents) {
				return
			}
		}
	}
}

func (cp ClientPool) handleClientChange(change ClientChange) {
	var err error = nil
	if change.added {
		log.Printf("adding client: %q", change.client)
		err = cp.addClient(change.client)
	} else {
		log.Printf("removing client: %q", change.client)
		err = cp.removeClient(change.client)
	}

	if err != nil {
		log.Println(err)
	}
	change.reply <- err == nil
}

func (cp ClientPool) addClient(client *ClientHandle) error {
	// Don't allow adding when quitting.
	if cp.quitting {
		return fmt.Errorf("addClient: quitting")
	}

	if _, ok := cp.contents[client]; ok {
		return fmt.Errorf("addClient: client %q already present", client)
	}
	cp.contents[client] = struct{}{}
	return nil
}

func (cp ClientPool) removeClient(client *ClientHandle) error {
	if _, ok := cp.contents[client]; ok {
		delete(cp.contents, client)
		return nil
	}
	return fmt.Errorf("removeClient: client %q not present", client)
}
