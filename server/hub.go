// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/gorilla/websocket"
)

type OnMessage func(fromAddr string, msg []byte)

// Client is a middleman between the websocket connection and the hub.
type RequestMsg struct {
	// The websocket connection.
	conn *websocket.Conn
	msg  []byte
}

// hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	request chan *RequestMsg

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func newHub() *Hub {
	return &Hub{
		request:    make(chan *RequestMsg),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) sendMessageToAddr(sendToIP string, message []byte) {
	// TODO: make this a hashtable to avoid iterating over all clients

	fmt.Println("Finding client to forward to...")
	for client := range h.clients {
		fmt.Println("..." + client.conn.RemoteAddr().String())
		if client.conn.RemoteAddr().String() == sendToIP {
			client.send <- message
			fmt.Println("...done")
			break
		}
	}
}

func (h *Hub) run(cb OnMessage) {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case req := <-h.request:
			cb(req.conn.RemoteAddr().String(), req.msg)
		}
	}
}
