package data

import (
	"fmt"
	"io"
	"github.com/multiformats/go-multiaddr"
	"github.com/libp2p/go-libp2p-peer"
	"github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p-crypto"

)
// Node .
type Node struct {
	Address    multiaddr.Multiaddr
	ID         peer.ID
	PS         peerstore.Peerstore
	Port       *int
	Incoming   chan *Message
	OutgoingID int
	MS         Store
}


func panicGuard(err error) {
	if err != nil {
		panic(err)
	}
}

// NewNode ..
func NewNode(r io.Reader, sourcePort *int) (node *Node) {
	node = new(Node)

	node.MS = make(Store)
	node.Incoming = make(chan *Message)
	prvKey, pubKey, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)

	panicGuard(err)

	node.ID, _ = peer.IDFromPublicKey(pubKey)
	if sourcePort != nil {
		node.Port = sourcePort
		node.Address, _ = multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", *node.Port))
	}

	node.PS = peerstore.NewPeerstore()
	node.PS.AddPrivKey(node.ID, prvKey)
	node.PS.AddPubKey(node.ID, pubKey)

	return
}