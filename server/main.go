package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"gnat"
	"log"
	"net/http"

	"strings"

	"time"

	"github.com/ccding/go-stun/stun"
	b58 "github.com/jbenet/go-base58"
)

var maxRouteCacheSize = 500
var addr = flag.String("localhost", ":2222", "http service address")
var dht *gnat.DHT
var hub *Hub
var forwardingCache map[string]*gnat.NetworkNode

func main() {
	// keep a route cache
	forwardingCache = make(map[string]*gnat.NetworkNode)

	// test the network to discovery what type of NAT (if any)
	// the client is behind.

	fmt.Println("GNAT Node v0.0.1")
	fmt.Println("  *Documentation:   https://gnat.cs.brown.edu/docs")
	fmt.Println("  *Support:         https://gnat.cs.brown.edu/support")
	fmt.Println("  *GitHub:          https://github.com/ogisan/gnat")
	fmt.Println("  For more information, visit: http://gnat.cs.brown.edu")
	fmt.Println("--------------------------------------------------------")
	fmt.Print("1) Testing network...")
	nat, host, err := stun.NewClient().Discover()
	if err != nil {
		fmt.Println("error:a problem occured while testing your network.")
		fmt.Println("TODO: try again later.")
	}

	fmt.Println("done")
	// acceptable type of NATs
	if nat == stun.NATNone || nat == stun.NATFull {
		fmt.Println("  Network NAT configuration: " + nat.String())
		fmt.Println("  Node address: " + host.String())
		initializeDHT()
		setupServer()
		fmt.Println("GNAT node setup and running!")
	} else {
		fmt.Println("error: your network configuration does not support running a GNAT node.")
		fmt.Println("TODO: update your router settings to have less restrictive settings and try again.")
	}
}

func sendMessageToClient(sendToIP string, message []byte) error {
	return hub.sendMessageToClient(sendToIP, message)
}

func forwardingRequestHandler(fromIP string, header map[string]string, data []byte) {

	ipDigest := sha256.Sum256([]byte(fromIP))
	id := b58.Encode(ipDigest[:])

	sendTo := header["to"]
	if sendTo == "" {
		return
	}

	fmt.Println("Received forwarding request from " + fromIP)

	msg := make(map[string]string)
	msg["from"] = fromIP
	msg["ts"] = time.Now().Format(time.RFC3339)
	msgHeader, _ := json.Marshal(msg)

	var err error
	var foundNode *gnat.NetworkNode
	if node, ok := forwardingCache[sendTo]; ok {
		foundNode = node
	} else {
		fmt.Println("Performing Kademlia.FIND_NODE operation")
		foundNode, err = dht.FindNode(id)
		forwardingCache[sendTo] = foundNode

		// if cache too big, reset it
		if len(forwardingCache) > maxRouteCacheSize {
			forwardingCache = make(map[string]*gnat.NetworkNode)
		}
	}

	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("Forwarding data to ", foundNode.IP.String())
		success := false
		success, err = dht.ForwardDataVia(foundNode, gnat.NewNetworkNode(sendTo, "0"), append(msgHeader, data...))
		fmt.Printf("Did forward: %v, %v\n", success, err)
	}
}

func handConnectionRequest(w http.ResponseWriter, r *http.Request) {

	// generate digest hash of IP address
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	ipDigest := sha256.Sum256([]byte(clientIP))
	id := b58.Encode(ipDigest[:])

	// find the node connected to this client ip
	node, err := dht.FindNode(id)

	if err == nil {
		if string(node.ID) == string(dht.GetSelfID()) {
			fmt.Println("New connection from " + r.RemoteAddr)
			//log.Println(r.URL)

			if r.URL.Path != "/" {
				http.Error(w, "Not found", 404)
				return
			}

			if r.Method != "GET" {
				http.Error(w, "Method not allowed", 405)
				return
			}

			http.ServeFile(w, r, "./static/home.html")

		} else {
			fmt.Println("Redirecting " + r.RemoteAddr + " to http://" + node.IP.String() + ":2222")
			http.Redirect(w, r, "http://"+node.IP.String()+":2222", 301)
		}

	} else {
		fmt.Println(err)
	}
}

func setupServer() {

	fmt.Print("4) Setting up HTTP server...")
	flag.Parse()
	hub = newHub()
	go hub.run(forwardingRequestHandler)
	http.HandleFunc("/", handConnectionRequest)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	fmt.Println("done")
	fmt.Println("Listening on http://127.0.0.1" + *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func initializeDHT() {
	var ip = flag.String("ip", "0.0.0.0", "IP Address to use")
	var port = flag.String("port", "1443", "Port to use")
	var bIP = flag.String("bip", "45.55.18.163", "IP Address to bootstrap against")
	var bPort = flag.String("bport", "1443", "Port to bootstrap against")
	var stun = flag.Bool("stun", true, "Use STUN")

	flag.Parse()

	var bootstrapNodes []*gnat.NetworkNode
	if *bIP != "" || *bPort != "" {
		bootstrapNode := gnat.NewNetworkNode(*bIP, *bPort)
		bootstrapNodes = append(bootstrapNodes, bootstrapNode)
	}

	var err error
	dht, err = gnat.NewDHT(&gnat.Options{
		BootstrapNodes:    bootstrapNodes,
		IP:                *ip,
		Port:              *port,
		UseStun:           *stun,
		ForwardingHandler: sendMessageToClient,
	})

	fmt.Print("2) Opening socket...")

	err = dht.CreateSocket()
	if err != nil {
		panic(err)
	}

	fmt.Println("done")
	go func() {
		err := dht.Listen()
		panic(err)
	}()

	if len(bootstrapNodes) > 0 {
		fmt.Print("3) Bootstrapping into GNAT network...")
		dht.Bootstrap()
		fmt.Println("done")
	} else {
		fmt.Println("3) Skipping GNAT bootstrap")
	}
}
