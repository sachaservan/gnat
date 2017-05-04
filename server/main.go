package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"gnat"
	"log"
	"net/http"

	"strings"

	"time"

	"os"

	"github.com/ccding/go-stun/stun"
	b58 "github.com/jbenet/go-base58"
)

// cap on how many routing nodes
// should be remembered
var maxRouteCacheSize = 500

// dht of peer GNAT nodes
var dht *gnat.DHT

// websocket hub to handle client messages
var hub *Hub

// mapping of ip addresses to gnat peers
var forwardingCache map[string]*gnat.NetworkNode

func main() {

	printWelcomeMessage()

	forwardingCache = make(map[string]*gnat.NetworkNode)

	// test the network to discovery what type of NAT (if any)
	// the client is behind.
	fmt.Print("1) Testing network...")
	nat, host, err := stun.NewClient().Discover()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: a problem occured while testing your network.")
		fmt.Fprintf(os.Stderr, "Try again later.")
		os.Exit(1)
	}
	fmt.Println("done.")

	var hostAddr string
	if host == nil {
		//out, _ := exec.Command("curl", "ipinfo.io/ip").Output()
		//hostAddr = strings.Split(string(out), "\n")[0] + ":1443"
	} else {
		hostAddr = host.String()
	}

	// make sure network is behind leniant NAT
	fmt.Println("  Network NAT configuration detected: " + nat.String())
	fmt.Println("  Public IP address: " + hostAddr)
	if nat == stun.NATNone || nat == stun.NATFull || nat == stun.NATPortRestricted || nat == stun.NATSymetricUDPFirewall || nat == stun.NATBlocked {

		// generate an ID from the digest of the IP addr
		selfIP := strings.Split(hostAddr, ":")[0]
		ipDigest := sha256.Sum256([]byte(selfIP))

		// initialize the GNAT dht node
		initializeDHT(ipDigest[:])

		// listen for clients
		setupServer("127.0.0.1:2222")

		fmt.Println("GNAT node running!")

	} else {
		fmt.Fprintf(os.Stderr, "error: your network configuration does not support running a GNAT node.")
		fmt.Fprintf(os.Stderr, "Update your router/firewall settings to be less restrictive and try again.")
		os.Exit(1)
	}
}

func sendMessageToClient(sendToIP string, message []byte) error {
	return hub.sendMessageToClient(sendToIP, message)
}

func aknowledgementHandler(success bool, errCode int, errMsg string) {
	fmt.Printf("Did forward: %v, %v\n", success, errMsg)
}

func forwardingRequestHandler(fromIP string, header map[string]string, data []byte) {

	sendTo := header["to"]
	if sendTo == "" {
		return
	}

	ipDigest := sha256.Sum256([]byte(sendTo))
	id := b58.Encode(ipDigest[:])

	fmt.Println("Received forwarding request from " + fromIP)

	msg := make(map[string]string)
	msg["from"] = fromIP
	msg["ts"] = time.Now().Format(time.RFC3339)
	msgHeader, _ := json.Marshal(msg)

	var err error
	var foundNode *gnat.NetworkNode
	if node, ok := forwardingCache[id]; ok {
		foundNode = node
	} else {
		fmt.Println("Performing Kademlia.FIND_NODE operation")
		foundNode, err = dht.FindNode(id)
		forwardingCache[id] = foundNode

		// if cache too big, reset it
		if len(forwardingCache) > maxRouteCacheSize {
			forwardingCache = make(map[string]*gnat.NetworkNode)
		}
	}

	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("Forwarding data to " + foundNode.IP.String())
		dht.ForwardDataVia(foundNode, gnat.NewNetworkNode(sendTo, "0"), append(msgHeader, data...), aknowledgementHandler)
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
		if bytes.Compare(node.ID, dht.GetSelfID()) == 0 {
			fmt.Println("New connection from " + r.RemoteAddr)

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

func setupServer(host string) {

	fmt.Print("4) Setting up HTTP server...")
	flag.Parse()
	hub = newHub()
	go hub.run(forwardingRequestHandler)
	http.HandleFunc("/", handConnectionRequest)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	fmt.Println("done")
	fmt.Println("Listening on " + host)
	err := http.ListenAndServe(":2222", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func initializeDHT(selfID []byte) {
	var ip = flag.String("ip", "0.0.0.0", "IP address to use")
	var port = flag.String("port", "1443", "Port to use")
	var bIP = flag.String("bip", "45.55.18.163", "IP address of bootstrap server")
	var bPort = flag.String("bport", "1443", "Port of bootstrap server")
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
		ID:                selfID,
		Port:              *port,
		UseStun:           *stun,
		ForwardingHandler: sendMessageToClient,
	})

	if err != nil {
		fmt.Println(err)
		return
	}

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

func printWelcomeMessage() {
	fmt.Println("\n  _____ _   _       _______  ")
	fmt.Println(" / ____| \\ | |   /\\|__   __|")
	fmt.Println("| |  __|  \\| |  /  \\  | |")
	fmt.Println("| | |_ | . ` | / /\\ \\ | |")
	fmt.Println("| |__| | |\\  |/ ____ \\| |")
	fmt.Println(" \\_____|_| \\_/_/    \\_\\_|")
	fmt.Println("  GNAT Server Node v0.0.1")
	fmt.Println("    *Documentation:   https://gnat.cs.brown.edu/docs")
	fmt.Println("    *Support:         https://gnat.cs.brown.edu/support")
	fmt.Println("    *GitHub:          https://github.com/ogisan/gnat")
	fmt.Println("    For more information, visit: http://gnat.cs.brown.edu")
}
