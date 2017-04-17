// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"strings"

	"fmt"

	"errors"

	"github.com/gorilla/websocket"
)

type onForwardDataReceived func(addr string, header map[string]string, data []byte)

// RequestMsg is a struct containing information
// to forward to client
type RequestMsg struct {
	// The websocket connection.
	conn *websocket.Conn
	msg  []byte
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[string]*Client

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
		clients:    make(map[string]*Client),
	}
}

func (h *Hub) sendMessageToClient(sendToIP string, message []byte) error {

	if client, ok := h.clients[sendToIP]; ok {
		fmt.Print("Sending data to client")
		client.send <- message
	} else {
		fmt.Println("\nError: client not connected")
		return errors.New("forward: " + sendToIP + " not connected")
	}

	return nil
}

func (h *Hub) run(cb onForwardDataReceived) {
	for {
		select {
		case client := <-h.register:
			clientIP := strings.Split(client.conn.RemoteAddr().String(), ":")[0]
			h.clients[clientIP] = client
		case client := <-h.unregister:
			clientIP := strings.Split(client.conn.RemoteAddr().String(), ":")[0]
			if _, ok := h.clients[clientIP]; ok {
				delete(h.clients, clientIP)
				close(client.send)
			}
		case req := <-h.request:
			header, data, err := parseRequest(req.msg)
			if err == nil {
				fromAddr := strings.Split(req.conn.RemoteAddr().String(), ":")[0]
				cb(fromAddr, header, data)
			}
		}
	}
}

func parseRequest(data []byte) (map[string]string, []byte, error) {

	headerLen := 0
	// find closing brace of header
	for i := 0; i < len(data); i++ {
		if string(data[i]) == "}" {
			headerLen = i + 1
			break
		}

		if i > 64 {
			return nil, nil, errors.New("bad request: header too large")
		}
	}

	header := map[string]string{}
	err := json.Unmarshal(data[:headerLen], &header)
	return header, data[headerLen:], err
}
