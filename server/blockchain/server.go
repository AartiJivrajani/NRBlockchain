package blockchain

import (
	"NonReplicated-Blockchain/common"
	"container/list"
	"context"
	"encoding/json"
	"net"
	"strconv"

	log "github.com/Sirupsen/logrus"
)

var (
	Server *BlockchainServer
)

type BlockchainServer struct {
	Port       int
	BlockChain *list.List
}

func CreateServer(ctx context.Context, port int) *BlockchainServer {

	return &BlockchainServer{Port: port, BlockChain: BlockChain}
}

func (server *BlockchainServer) Start(ctx context.Context) {
	var (
		err error
	)
	log.WithFields(log.Fields{
		"port": server.Port,
	}).Debug("Starting the blockchain server....")

	PORT := ":" + strconv.Itoa(server.Port)
	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		log.Panic("error starting the server, shutting down...")
		return
	}
	defer listener.Close()

	for {
		c, err := listener.Accept()
		if err != nil {
			log.Error("error connecting to the TCP socket to initiate the read, shutting down...")
			return
		}
		go server.handleConnections(ctx, c)
	}
}

func (server *BlockchainServer) handleConnections(ctx context.Context, c net.Conn) {
	var (
		requestMsg *common.ServerRequest
		err        error
	)
	d := json.NewDecoder(c)
	for {
		err = d.Decode(&requestMsg)
		if err != nil {
			continue
		}
		log.WithFields(log.Fields{
			"request": requestMsg,
		}).Debug("Request recvd from a client")
		go server.serveRequest(ctx, c, requestMsg)
	}
}

func (server *BlockchainServer) serveRequest(ctx context.Context, c net.Conn, requestMsg *common.ServerRequest) {
	var (
		serverResp *common.ServerResponse
		resp       []byte
	)

	switch requestMsg.TxnType {
	case common.BalanceTxn:
		serverResp, _ = server.BalanceTransaction(ctx, requestMsg)
		resp, _ = json.Marshal(serverResp)
		log.WithFields(log.Fields{
			"resp":      serverResp,
			"client_id": requestMsg.ClientId,
		}).Debug("sending resp back to client")
		c.Write(resp)
	case common.TransferTxn:
		serverResp, _ = server.TransferTransaction(ctx, requestMsg)
		resp, _ = json.Marshal(serverResp)
		log.WithFields(log.Fields{
			"resp":      serverResp,
			"client_id": requestMsg.ClientId,
		}).Debug("sending resp back to client")
		c.Write(resp)
	}
}
