package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"

	"github.com/libp2p/go-libp2p-net"
	dat "github.com/qballer/libp2p-examples/data"
)


func panicGuard(err error) {
	if err != nil {
		panic(err)
	}
}

func handleGossipStream(node *dat.Node) (handler net.StreamHandler) {
	return func(s net.Stream) {
		log.Println("Got a new stream!")
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		node.PS.Put(s.Conn().RemotePeer(), dat.Stream, rw)
		go readData(rw, node)
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

	node := dat.NewNode(r, sourcePort, handleGossipStream)
	
	rw := node.ConnectToDest(dest)
	if rw != nil {
		go readData(rw, node)
	}
	

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
			fmt.Printf("%s > %s", messageKey, message.Message)
			go node.SendToPeers(message)
		}
	}
}


// ---- go routines functions ----

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
		msg := dat.CreateMessage(sendData, node.Address.String(), node.OutgoingID)
		node.MS[msg.Key()] = msg
		node.SendToPeers(msg)
	}
}