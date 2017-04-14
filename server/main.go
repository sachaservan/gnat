package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"gnat"
	"log"
	"net/http"

	b58 "github.com/jbenet/go-base58"
)

var addr = flag.String("localhost", ":80", "http service address")
var dht *gnat.DHT
var hub *Hub

func main() {
	initializeDHT()
	setupServer()
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

	fmt.Println("Received forwarding request from " + fromAddr)

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
			fmt.Println("Client accepted by " + node.IP.String())
			log.Println(r.URL)

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
			fmt.Println("Redirecting to http:/" + node.IP.String())
			http.Redirect(w, r, "http:/"+node.IP.String(), 301)
		}

	} else {
		fmt.Println(err)
	}
}

func setupServer() {
	flag.Parse()
	hub = newHub()
	go hub.run(onForwardData)
	http.HandleFunc("/", handConnectionRequest)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	fmt.Println("Waiting for clients on " + *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func initializeDHT() {
	var ip = flag.String("ip", "0.0.0.0", "IP Address to use")
	var port = flag.String("port", "2222", "Port to use")
	var bIP = flag.String("bip", "", "IP Address to bootstrap against")
	var bPort = flag.String("bport", "", "Port to bootstrap against")
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

	fmt.Println("Opening socket..")

	if *stun {
		fmt.Println("Discovering public address using STUN..")
	}

	err = dht.CreateSocket()
	if err != nil {
		panic(err)
	}
	fmt.Println("..done")

	go func() {
		fmt.Println("GNAT Kademlia listening on " + dht.GetNetworkAddr())
		err := dht.Listen()
		panic(err)
	}()

	if len(bootstrapNodes) > 0 {
		fmt.Println("Bootstrapping..")
		dht.Bootstrap()
		fmt.Println("..done")
	}
}

func forwardMessage(ip string, msg []byte) {
	ipDigest := sha256.Sum256([]byte(ip))
	id := b58.Encode(ipDigest[:])
	fmt.Println("Searching for forwarding node...")
	node, err := dht.FindNode(id)

	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("..forwarding node found:", node.IP.String())
		dht.ForwardData(node, gnat.NewNetworkNode(ip, "0"), msg)
	}
}
