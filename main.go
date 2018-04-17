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

	"github.com/libp2p/go-libp2p-crypto"
	"github.com/libp2p/go-libp2p-host"
	"github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-peer"
	"github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p-swarm"
	"github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/multiformats/go-multiaddr"
)

const stream = "stream"
const id = "ID"

func panicGuard(err error) {
	if err != nil {
		panic(err)
	}
}

// Node .
type Node struct {
	address    multiaddr.Multiaddr
	id         peer.ID
	ps         peerstore.Peerstore
	port       *int
	incoming   chan *Message
	outgoingID int
	ms         MessageStore
}

// Message ..
type Message struct {
	message string
	id      int
	origin  string
}

// MessageStore ...
type MessageStore map[string]*Message

func createNode(r io.Reader, sourcePort *int) (node *Node) {
	node = new(Node)

	node.ms = make(MessageStore)
	node.incoming = make(chan *Message)
	prvKey, pubKey, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)

	panicGuard(err)

	node.id, _ = peer.IDFromPublicKey(pubKey)
	if sourcePort != nil {
		node.port = sourcePort
		node.address, _ = multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", *node.port))
	}

	node.ps = peerstore.NewPeerstore()
	node.ps.AddPrivKey(node.id, prvKey)
	node.ps.AddPubKey(node.id, pubKey)

	return
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

func connectToDest(dest *string, host *basichost.BasicHost, node *Node) {
	if *dest == "" {
		return
	}

	peerID := addAddrToPeerstore(host, *dest)

	fmt.Println("This node's multiaddress: ")
	fmt.Printf("%s/ipfs/%s\n", node.address, host.ID().Pretty())

	s, err := host.NewStream(context.Background(), peerID, "/chat/1.0.0")

	panicGuard(err)

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
	node.ps.Put(s.Conn().RemotePeer(), stream, rw)

	go readData(rw, node)
}

func createHost(node *Node) *basichost.BasicHost {

	network, err := swarm.NewNetwork(context.Background(), []multiaddr.Multiaddr{node.address}, node.id, node.ps, nil)
	panicGuard(err)

	host := basichost.New(network)
	host.SetStreamHandler("/chat/1.0.0", handleGossipStream(node))

	fmt.Println("Incomming address:")
	fmt.Printf("/ip4/127.0.0.1/tcp/%d/ipfs/%s\n", *node.port, host.ID().Pretty())
	fmt.Printf("\nWaiting for incoming connection\n\n")
	return host
}

func handleGossipStream(node *Node) (handler net.StreamHandler) {
	return func(s net.Stream) {
		log.Println("Got a new stream!")
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		node.ps.Put(s.Conn().RemotePeer(), stream, rw)
		go readData(rw, node)
	}
}

func readData(rw *bufio.ReadWriter, node *Node) {
	for {
		decoder := json.NewDecoder(rw.Reader)
		message := new(Message)
		err := decoder.Decode(message)
		if err != nil {
			fmt.Println("error!!!!")
			fmt.Println(err.Error())
			return
		}
		message.writeData("got-mssage", message)
		node.incoming <- message
	}
}

func writeData(node *Node) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')

		panicGuard(err)

		peers := node.ps.Peers()
		for _, peer := range peers {
			if peer != node.id {
				remoteRW, _ := node.ps.Get(peer, stream)
				rw := remoteRW.(*bufio.ReadWriter)
				encoder := json.NewEncoder(rw.Writer)
				node.outgoingID++

				message := new(Message)
				message.message = sendData
				message.id = node.outgoingID
				message.origin = node.address.String()

				message.writeData("created message", message)

				node.ms[message.Key()] = message
				err := encoder.Encode(message)
				panicGuard(err)

			}
		}
	}

}

func (m *Message) writeData (pre string, message *Message) {
	fmt.Println("---------------------")
	fmt.Printf("*** %s ***\n", pre)
	fmt.Printf("id:%d\n origin:%s\n message:%s", message.id, message.origin, message.message)
	fmt.Println("---------------------")

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

	node := createNode(r, sourcePort)

	host := createHost(node)
	connectToDest(dest, host, node)

	go writeData(node)
	go handleIncoming(node)
	select {}
}

// Key ..
func (m *Message) Key() string {
	return fmt.Sprintf("%s|%d", m.origin, m.id)
}

func handleIncoming(node *Node) {
	for {
		fmt.Println("listening:!")
		message := <-node.incoming
		fmt.Println("got it!")
		messageKey := message.Key()
		exist := node.ms[messageKey]
		if exist == nil {
			node.ms[messageKey] = message
			fmt.Printf("%s > %s", messageKey, message.message)
		}
	}
}
