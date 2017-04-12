package main

import (
	"crypto/sha256"
	"encoding/json"
	"gnat"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	b58 "github.com/jbenet/go-base58"
)

var addr = flag.String("localhost", ":8080", "http service address")
var dht *gnat.DHT
var hub *Hub

func main() {
	initializeDHT()
	setupServer()
}

func onForwardRequestReceived(forwardToIP string, msg []byte) {
	hub.sendMessageToAddr(forwardToIP, msg)
}

func onClientMessageReceived(addr string, message []byte) {
	u := map[string]string{}
	resp := map[string]string{}
	json.Unmarshal(message, &u)
	clientIP := strings.Split(addr, ":")[0]

	msgType := u["type"]
	if msgType == "" {
		return
	}

	switch msgType {

	case "RQST_CONNECT":
		fmt.Println("Received connection request from " + addr)

		// generate digest hash of IP address
		ipDigest := sha256.Sum256([]byte(clientIP))
		id := b58.Encode(ipDigest[:])

		// find the node connected to this client ip
		node, err := dht.FindNode(id)

		if err == nil {
			if string(node.ID) == string(dht.GetSelfID()) {
				fmt.Println("Sending connection accepted message")
				resp["type"] = "CONNECTION_ACCEPTED"
			} else {
				fmt.Println("Sending redirect message")
				resp["type"] = "REDIRECT"
				resp["redirect"] = node.IP.String() + ":" + strconv.Itoa(node.Port)
			}

			respMsg, _ := json.Marshal(resp)
			hub.sendMessageToAddr(clientIP, respMsg)
		} else {
			fmt.Println(err)
		}
		break
	case "RQST_FORWARD":
		fmt.Println("Received forwarding request from " + addr)

		sendTo := u["sendTo"]
		if !strings.Contains(sendTo, ":") {
			// invalid ip address format
			resp["error"] = "Bad request"
			respMsg, _ := json.Marshal(resp)
			hub.sendMessageToAddr(clientIP, respMsg)
			break
		}
		sendToIP := strings.Split(sendTo, ":")[0]
		sendToPort := strings.Split(sendTo, ":")[1]

		resp["sender"] = addr
		resp["data"] = u["data"]
		respMsg, _ := json.Marshal(resp)
		forwardMessage(sendToIP, sendToPort, respMsg)
		break
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", 404)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func setupServer() {
	flag.Parse()
	hub = newHub()
	go hub.run(onClientMessageReceived)
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func initializeDHT() {
	var ip = flag.String("ip", "0.0.0.0", "IP Address to use")
	var port = flag.String("port", "8080", "Port to use")
	var bIP = flag.String("bip", "", "IP Address to bootstrap against")
	var bPort = flag.String("bport", "", "Port to bootstrap against")
	var stun = flag.Bool("stun", true, "Use STUN")

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
		fmt.Println("Now listening on " + dht.GetNetworkAddr())
		err := dht.Listen()
		panic(err)
	}()

	if len(bootstrapNodes) > 0 {
		fmt.Println("Bootstrapping..")
		dht.Bootstrap()
		fmt.Println("..done")
	}
}

func forwardMessage(ip string, port string, msg []byte) {
	ipDigest := sha256.Sum256([]byte(ip))
	id := b58.Encode(ipDigest[:])
	fmt.Println("Searching for forwarding node: [" + ip + ": " + id + "]")
	node, err := dht.FindNode(id)

	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("..forwarding node found:", node.IP.String())
		dht.ForwardData(node, gnat.NewNetworkNode(ip, port), msg)
	}
}
