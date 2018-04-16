package main 

import (
	"log"
	"fmt"
	"flag"
	"io"
	"context"
	mrand "math/rand"
	"crypto/rand"
	"bufio"
	"os"
	
	"github.com/libp2p/go-libp2p-crypto"
	"github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p-peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/libp2p/go-libp2p-swarm"
	"github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-host"

)

const stream = "stream"
const id = "ID"

func panicGuard (err error) {
	if err != nil {
		panic(err)
	}
}
// Node .
type Node struct{
	address multiaddr.Multiaddr
	id peer.ID
	ps peerstore.Peerstore
	port *int
	incoming chan string
	outgoingID int 
}

func createNode(r io.Reader, sourcePort *int) (node *Node ){
	node = new(Node)
	node.port = sourcePort
	prvKey, pubKey, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
 
	 panicGuard(err)
 
	 node.id, _ = peer.IDFromPublicKey(pubKey)
 
	 node.address, _ = multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *sourcePort))
 
	 node.ps = peerstore.NewPeerstore()
	 err1 := node.ps.AddPrivKey(node.id, prvKey)
	 err2 := node.ps.AddPubKey(node.id, pubKey)

	 panicGuard(err1)
	 panicGuard(err2)
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

func createHost(node *Node) (*basichost.BasicHost) {
	
	network, err := swarm.NewNetwork(context.Background(), []multiaddr.Multiaddr{node.address}, node.id, node.ps, nil)
	panicGuard(err)

	host := basichost.New(network)
	host.SetStreamHandler("/chat/1.0.0", handleGossipStream(node))

	fmt.Printf("Run './chat -d /ip4/127.0.0.1/tcp/%d/ipfs/%s' on another console.\n You can replace 127.0.0.1 with public IP as well.\n", *node.port, host.ID().Pretty())
	fmt.Printf("\nWaiting for incoming connection\n\n")
	return host
}

func handleGossipStream(node *Node) (handler net.StreamHandler){ 
	return func (s net.Stream) {
		log.Println("Got a new stream!")
	
		// Create a buffer stream for non blocking read and write.
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		
		node.ps.Put(s.Conn().RemotePeer(), stream, rw)
		
		go readData(rw, node)
	}
}


func readData(rw *bufio.ReadWriter, node *Node) {
	for {
		str, _ := rw.ReadString('\n')

		if str == "" {
			return
		}
		if str != "\n" {
			// Green console colour: 	\x1b[32m
			// Reset console colour: 	\x1b[0m
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)
		}

	}
}

func writeData(node *Node) {
	stdReader := bufio.NewReader(os.Stdin)
	
	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')

		panicGuard(err)

		peers := node.ps.Peers()
		for  _, peer := range peers {
			if peer != node.id {
				remoteRW, _ := node.ps.Get(peer, stream)
				rw := remoteRW.(*bufio.ReadWriter)
				rw.WriteString(fmt.Sprintf("%s\n", sendData))
				rw.Flush()
			}
		}
	}

}

func main (){
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
	select {}
}

