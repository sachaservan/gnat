package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"gnat"
	"log"
	"net/http"

	"github.com/ccding/go-stun/stun"
	b58 "github.com/jbenet/go-base58"
)

var addr = flag.String("localhost", ":2222", "http service address")
var dht *gnat.DHT
var hub *Hub

func main() {
	// test the network to discovery what type of NAT (if any)
	// the client is behind.

	fmt.Println("GNAT Node v0.0.1")
	fmt.Println("  *Documentation:   https://gnat.cs.brown.edu/docs")
	fmt.Println("  *Support:  	     https://gnat.cs.brown.edu/support")
	fmt.Println("  *GitHub:  	     https://github.com/ogisan/gnat")
	fmt.Println("  For more information, visit: http://gnat.cs.brown.edu")
	fmt.Println("--------------------------------------------------------")
	fmt.Print("1) Testing network...")
	nat, host, err := stun.NewClient().Discover()
	if err != nil {
		fmt.Println("Error:a problem occured while testing your network.")
		fmt.Println("TODO: try again later.")
	}

	fmt.Println("done")
	// acceptable type of NATs
	if nat == stun.NATNone || nat == stun.NATFull {
		fmt.Println("Network NAT configuration: " + nat.String())
		fmt.Println("Node address: " + host.String())
		initializeDHT()
		setupServer()
		fmt.Println("GNAT node setup and running!")
	} else {
		fmt.Println("Error: your network configuration does not support running a GNAT node.")
		fmt.Println("TODO: update your router settings to have less restrictive settings and try again.")
	}
}

func onForwardRequestReceived(forwardToIP string, rqst []byte) {
	hub.sendMessageToAddr(forwardToIP, rqst)
}

func onForwardData(fromAddr string, header map[string]string, data []byte) {

	resp := map[string]string{}

	sendTo := header["send_to"]
	if sendTo == "" {
		return
	}

	fmt.Println("Forward request from " + fromAddr)

	resp["from"] = fromAddr
	respHeader, _ := json.Marshal(resp)
	forwardMessage(sendTo, append(respHeader, data...))
}

func handConnectionRequest(w http.ResponseWriter, r *http.Request) {

	// generate digest hash of IP address
	ipDigest := sha256.Sum256([]byte(r.RemoteAddr))
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
			fmt.Println("Redirecting " + r.RemoteAddr + " to http:/" + node.IP.String())
			http.Redirect(w, r, "http:/"+node.IP.String(), 301)
		}

	} else {
		fmt.Println(err)
	}
}

func setupServer() {

	fmt.Print("4) Setting up HTTP server...")
	flag.Parse()
	hub = newHub()
	go hub.run(onForwardData)
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
	var stun = flag.Bool("stun", false, "Use STUN")

	flag.Parse()

	var bootstrapNodes []*gnat.NetworkNode
	if *bIP != "" || *bPort != "" {
		bootstrapNode := gnat.NewNetworkNode(*bIP, *bPort)
		bootstrapNodes = append(bootstrapNodes, bootstrapNode)
	}

	var err error
	dht, err = gnat.NewDHT(&gnat.Options{
		BootstrapNodes:   bootstrapNodes,
		IP:               *ip,
		Port:             *port,
		UseStun:          *stun,
		OnForwardRequest: onForwardRequestReceived,
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
	}
}

func forwardMessage(ip string, msg []byte) {
	ipDigest := sha256.Sum256([]byte(ip))
	id := b58.Encode(ipDigest[:])
	fmt.Print("Finding forwarding node...")
	node, err := dht.FindNode(id)
	fmt.Println("done")
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("Forwarding data to ", node.IP.String())
		dht.ForwardData(node, gnat.NewNetworkNode(ip, "0"), msg)
	}
}
