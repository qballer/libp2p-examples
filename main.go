package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"

	"github.com/libp2p/go-libp2p-host"
	"github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-peer"
	"github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p-swarm"
	"github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/multiformats/go-multiaddr"
	dat "github.com/qballer/libp2p-examples/data"
)

const stream = "stream"
const id = "ID"

func panicGuard(err error) {
	if err != nil {
		panic(err)
	}
}

func addAddrToPeerstore(h host.Host, addr string) peer.ID {
	ipfsaddr, err := multiaddr.NewMultiaddr(addr)
	panicGuard(err)

	pid, err := ipfsaddr.ValueForProtocol(multiaddr.P_IPFS)
	panicGuard(err)

	peerid, err := peer.IDB58Decode(pid)
	panicGuard(err)

	targetPeerAddr, _ := multiaddr.NewMultiaddr(
		fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(peerid)))
	targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

	h.Peerstore().AddAddr(peerid, targetAddr, peerstore.PermanentAddrTTL)
	return peerid
}

func connectToDest(dest *string, host *basichost.BasicHost, node *dat.Node) {
	if *dest == "" {
		return
	}

	peerID := addAddrToPeerstore(host, *dest)

	fmt.Println("This node's multiaddress: ")
	fmt.Printf("%s/ipfs/%s\n", node.Address, host.ID().Pretty())

	s, err := host.NewStream(context.Background(), peerID, "/chat/1.0.0")

	panicGuard(err)

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
	node.PS.Put(s.Conn().RemotePeer(), stream, rw)

	go readData(rw, node)
}

func createHost(node *dat.Node) *basichost.BasicHost {

	network, err := swarm.NewNetwork(context.Background(), []multiaddr.Multiaddr{node.Address}, node.ID, node.PS, nil)
	panicGuard(err)

	host := basichost.New(network)
	host.SetStreamHandler("/chat/1.0.0", handleGossipStream(node))

	fmt.Println("Incomming address:")
	fmt.Printf("/ip4/127.0.0.1/tcp/%d/ipfs/%s\n", *node.Port, host.ID().Pretty())
	fmt.Printf("\nWaiting for incoming connection\n\n")
	return host
}

func handleGossipStream(node *dat.Node) (handler net.StreamHandler) {
	return func(s net.Stream) {
		log.Println("Got a new stream!")
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		node.PS.Put(s.Conn().RemotePeer(), stream, rw)
		go readData(rw, node)
	}
}

func readData(rw *bufio.ReadWriter, node *dat.Node) {
	for {
		decoder := json.NewDecoder(rw.Reader)
		message := new(dat.Message)
		err := decoder.Decode(message)
		if err != nil {
			fmt.Println("error!!!!")
			fmt.Println(err.Error())
			return
		}
		node.Incoming <- message
	}
}

func writeData(node *dat.Node) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')

		panicGuard(err)
		node.OutgoingID++

		msg := createMessage(sendData, node.Address.String(), node.OutgoingID)
		sendToPeers(msg, node.PS.Peers(), node)
	}
}

func createMessage(data string, address string, id int) *dat.Message {
	message := new(dat.Message)
	message.Message = data
	message.ID = id
	message.Origin = address
	return message

}

func sendToPeers(message *dat.Message, peers []peer.ID, node *dat.Node) {
	for _, peer := range peers {
		if peer != node.ID {
			remoteRW, _ := node.PS.Get(peer, stream)
			rw := remoteRW.(*bufio.ReadWriter)
			enc := json.NewEncoder(rw.Writer)

			node.MS[message.Key()] = message
			err := enc.Encode(*message)
			rw.Flush()
			panicGuard(err)
		}
	}
}

func main() {
	sourcePort := flag.Int("sp", 0, "Source port number")
	dest := flag.String("d", "", "Dest MultiAddr String")
	debug := flag.Bool("debug", true, "Debug generated same node id on every execution.")

	flag.Parse()

	var r io.Reader
	if *debug {
		r = mrand.New(mrand.NewSource(int64(*sourcePort)))
	} else {
		r = rand.Reader
	}

	node := dat.NewNode(r, sourcePort)

	host := createHost(node)
	connectToDest(dest, host, node)

	go writeData(node)
	go handleIncoming(node)
	select {}
}

func handleIncoming(node *dat.Node) {
	for {
		message := <-node.Incoming
		messageKey := message.Key()
		exist := node.MS[messageKey]
		if exist == nil {
			node.MS[messageKey] = message
			fmt.Printf("%s > %s", messageKey, message.Message)
			sendToPeers(message, node.PS.Peers(), node)
		}
	}
}
