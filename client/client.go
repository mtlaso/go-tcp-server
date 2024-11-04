package client

import (
	"net"
	"slices"
	"sync"
	"sync/atomic"
)

// Clients represents the Clients connected to the server.
type Clients struct {
	Clients             map[int64]net.Conn
	WaitingQueueIDs     []int64
	Mu                  sync.RWMutex
	MaxConnectedClients int
	// Do NOT! change this field directly!
	ID int64
}

// AddToWaitingQueue adds a client to the waiting queue.
func (c *Clients) AddToWaitingQueue(clientID int64) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.WaitingQueueIDs = append(c.WaitingQueueIDs, clientID)
}

// UpdateWaitingQueue removes a client from the waiting queue, either by its clientID
// if the client that just left was inside the waiting queue,
// or removes the next client in the waiting queue (first index in slice).
//
// If clientID is -1, it will remove the next client from the waiting queue.
func (c *Clients) UpdateWaitingQueue(clientID int64) {
	idx := c.IndexClientInWaitingQueue(clientID)

	c.Mu.Lock()
	if len(c.WaitingQueueIDs) != 0 {
		if idx == -1 {
			_, c.WaitingQueueIDs = c.WaitingQueueIDs[0], c.WaitingQueueIDs[1:]
		} else {
			c.WaitingQueueIDs = slices.DeleteFunc(c.WaitingQueueIDs, func(el int64) bool {
				return el == clientID
			})
		}
	}
	c.Mu.Unlock()
}

// IndexClientInWaitingQueue returns the index of the client, by it's clientID, if it's inside the waiting queue.
// If the clientID is not inside the waiting queue, it returns -1.
func (c *Clients) IndexClientInWaitingQueue(clientID int64) int {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return slices.Index(c.WaitingQueueIDs, clientID)
}

// Add adds a client to the clients list.
func (c *Clients) Add(id int64, conn net.Conn) {
	c.Mu.Lock()
	c.Clients[id] = conn
	c.Mu.Unlock() // Unlock now to avoid deadlock inside addToWaitingQueue().

	if len(c.Clients) > c.MaxConnectedClients {
		c.AddToWaitingQueue(id)
	}
}

// Remove removes a client from the clients list and updates the waiting queue.
func (c *Clients) Remove(clientID int64) {
	c.Mu.Lock()
	delete(c.Clients, clientID)
	c.Mu.Unlock() // Unlock now to prevent deadlock.

	c.UpdateWaitingQueue(clientID)
}

// Count returns the number of clients connected to the server.
func (c *Clients) Count() int {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return len(c.Clients)
}

// NextID returns the next id of a client.
func (c *Clients) NextID() int64 {
	return atomic.AddInt64(&c.ID, 1)
}
