package consensus

import (
	"NRBlockchain/common"
	"NRBlockchain/lamport"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/manifoldco/promptui"
)

var (
	Client           *BlockchainClient
	GlobalClock      = 0
	consensusMsgChan = make(chan *common.ConsensusEvent)
	numMsg           = 0
	allDoneChan      = make(chan bool)
	transactionStart = make(chan bool)
)

// TODO: See if the client id and the lamport clock pid can be made the same for now
type BlockchainClient struct {
	ClientId    int
	PortNumber  int
	Clock       *lamport.LamportClock
	Q           []int
	Peers       []int
	PeerConnMap map[int]net.Conn
}

func GetClient(ctx context.Context, clientId int, portNumber int) *BlockchainClient {
	return &BlockchainClient{
		Clock: &lamport.LamportClock{
			Timestamp: 0,
			PID:       0,
		},
		Q:           make([]int, 3),
		ClientId:    clientId,
		PortNumber:  portNumber,
		Peers:       make([]int, 2),
		PeerConnMap: make(map[int]net.Conn),
	}
}

func (client *BlockchainClient) handlePeerMessages(ctx context.Context, conn net.Conn) {
	var (
		resp *common.ConsensusEvent
		err  error
	)
	d := json.NewDecoder(conn)
	for {
		err = d.Decode(&resp)
		if err != nil {

		}
		consensusMsgChan <- resp
	}
}

func (client *BlockchainClient) handlePeerConnections(ctx context.Context, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {

		}
		go client.handlePeerMessages(ctx, conn)
	}
}

// startPeerListener starts a listener on its socket and listens for any messages it may get
func (client *BlockchainClient) startPeerListener(ctx context.Context) {
	var (
		err      error
		listener net.Listener
		PORT     string
	)
	PORT = ":" + strconv.Itoa(common.ClientPortMap[client.ClientId])
	listener, err = net.Listen("tcp", PORT)
	if err != nil {
		log.WithFields(log.Fields{
			"error":       err.Error(),
			"client_id":   client.ClientId,
			"client_port": common.ClientPortMap[client.ClientId],
		}).Error("error starting a listener on the port")
		return
	}
	go client.handlePeerConnections(ctx, listener)
	transactionStart <- true
}

func (client *BlockchainClient) updateGlobalClock(ctx context.Context, msg *common.ConsensusEvent) {
	msgClock := msg.CurrentClock.Timestamp
	// TODO: Take care of the case when msgClock == GlobalClock
	if msgClock > GlobalClock {
		GlobalClock = msgClock + 1
	} else {
		GlobalClock += 1
	}
}

// processClientMessages essentially gathers all the messages a client may receive...
// this could be ACK, RELEASE, REQUEST messages
func (client *BlockchainClient) processClientMessages(ctx context.Context) {
	for {
		select {
		case msg := <-consensusMsgChan:
			if msg.Message == common.Ack {
				numMsg += 1
				if numMsg >= 2 {
					allDoneChan <- true
				}
			} else if msg.Message == common.Request {
				// process the request received from the other clients
				client.Q = append(client.Q, msg.SourceClient)
				client.updateGlobalClock(ctx, msg)

				// send an ACK message to the requesting client.
				msg := &common.ConsensusEvent{
					Message:      common.Ack,
					SourceClient: client.ClientId,
					DestClient:   msg.SourceClient,
					CurrentClock: &lamport.LamportClock{
						Timestamp: GlobalClock,
						PID:       client.ClientId,
					},
				}
				client.sendSingleMessage(ctx, client.PeerConnMap[msg.SourceClient], msg)
			} else if msg.Message == common.Release {
				// TODO: Take care of release messages
			}
		case <-allDoneChan:
			numMsg = 0
			return
		}
	}
}

// Start essentially does the following -
// 1. Register the rest of the clients with the current client
// 2. Establishes a connection to each of the client
// 3. Logs the essential information needed
func (client *BlockchainClient) Start(ctx context.Context) {
	client.registerClients(ctx)
	client.establishPeerConnections(ctx)
	go client.startPeerListener(ctx)

	// wait for the peer listener to start before we allow the transactions to begin
	<-transactionStart

	go client.processClientMessages(ctx)
	go client.startTransactions(ctx)
}

// registerClients detects each peer that it has and stores its connection object
func (client *BlockchainClient) registerClients(ctx context.Context) {

	log.WithFields(log.Fields{
		"clientId":    client.ClientId,
		"clientClock": client.Clock.Timestamp,
	}).Debug("Registering the clients")

	if client.ClientId == 0 {
		client.Peers = append(client.Peers, []int{1, 2}...)
	} else if client.ClientId == 1 {
		client.Peers = append(client.Peers, []int{0, 2}...)
	} else if client.ClientId == 2 {
		client.Peers = append(client.Peers, []int{0, 1}...)
	}
}

// UI
// startTransactions keeps a connection alive in order to receive any events from the rest of the clients
func (client *BlockchainClient) startTransactions(ctx context.Context) {
	var (
		err                       error
		receiverClient, amountStr string
		amount                    float64
		transactionType           string
		receiverClientId          int
		txn                       string
	)
	for {
		prompt := promptui.Select{
			Label: "Select Transaction",
			Items: []string{"Show Balance", "Transfer", "Exit"},
		}

		_, transactionType, err = prompt.Run()
		if err != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Error("error fetching transaction type from the command line")
			continue
		}
		log.WithFields(log.Fields{
			"choice": transactionType,
		}).Debug("You choose...")
		switch transactionType {
		case "Exit":
			log.Debug("Fun doing business with you, see you soon!")
			return
		case "Show Balance":
			log.Debug("sending request to server....")
			txn = common.BalanceTxn
			client.sendRequestToServer(ctx, &common.ServerRequest{
				TxnType:  txn,
				ClientId: client.ClientId,
			})
		case "Transfer":
			txn = common.TransferTxn
			prompt := promptui.Prompt{
				Label: "Receiver Client",
				//Validate: validate,
			}
			receiverClient, err = prompt.Run()
			if err != nil {
				log.WithFields(log.Fields{
					"error": err.Error(),
				}).Error("error fetching the client number from the command line")
				continue
			}
			receiverClientId, _ = strconv.Atoi(receiverClient)
			prompt = promptui.Prompt{
				Label:   "Amount to be transacted",
				Default: "",
			}
			amountStr, err = prompt.Run()
			if err != nil {
				log.WithFields(log.Fields{
					"error": err.Error(),
				}).Error("error fetching the transaction amount from the command line")
				continue
			}
			amount, _ = strconv.ParseFloat(amountStr, 64)
			client.sendRequestToServer(ctx, &common.ServerRequest{
				TxnType:  txn,
				ClientId: client.ClientId,
				Amount:   amount,
				Rcvr:     receiverClientId,
			})
		}
	}
}

func (client *BlockchainClient) establishPeerConnections(ctx context.Context) {
	var (
		err  error
		conn net.Conn
	)
	log.WithFields(log.Fields{
		"client_id": client.ClientId,
	}).Debug("establishing peer connections with other clients")
	for peer := range client.Peers {
		PORT := ":" + strconv.Itoa(common.ClientPortMap[peer])
		conn, err = net.Dial("tcp", PORT)
		if err != nil {
			log.WithFields(log.Fields{
				"error":          err.Error(),
				"client_id":      client.ClientId,
				"peer_client_id": peer,
			}).Error("error connecting to the client")
			// TODO: backoff and retry?
		}
		client.PeerConnMap[peer] = conn
	}
}

// getConsensus sends a request to each client and waits for an ACK from each of them.
// once it receives an ACK from all the clients, it checks if it is at the head of the Priority Queue
// if yes, it makes a request to the blockchain server followed by multi-casting a release message
// to all the clients.
func (client *BlockchainClient) GetConsensus(ctx context.Context) error {
	var (
		consensusReq *common.ConsensusEvent
		err          error
		cReq         []byte
	)
	consensusReq = &common.ConsensusEvent{
		Message:      common.Request,
		SourceClient: client.ClientId,
		DestClient:   0,
		// TODO: Populate this clock correctly
		CurrentClock: nil,
	}

	for peer := range client.Peers {
		if client.PeerConnMap[peer] == nil {
			log.WithFields(log.Fields{
				"client_id":      client.ClientId,
				"peer_client_id": peer,
			}).Panic("no pre-established connection found, exiting now...")
			return fmt.Errorf("no pre-established connection found")
		}
		consensusReq.DestClient = peer
		cReq, err = json.Marshal(consensusReq)
		_, err = client.PeerConnMap[peer].Write(cReq)
		if err != nil {

		}
	}
	if err != nil {
		log.Error(err.Error())
		return fmt.Errorf(err.Error())
	}
	return nil
}

func (client *BlockchainClient) sendSingleMessage(ctx context.Context, conn net.Conn, msg *common.ConsensusEvent) {

}

func (client *BlockchainClient) sendRequestToServer(ctx context.Context, request *common.ServerRequest) {
	var (
		err  error
		conn net.Conn
		jReq []byte
		resp *common.ServerResponse
	)
	err = client.GetConsensus(ctx)
	if err != nil {
		log.Error("Consensus not reached")
		return
	}
	PORT := ":" + strconv.Itoa(common.ServerPort)
	conn, err = net.Dial("tcp", PORT)
	if err != nil {
		log.WithFields(log.Fields{
			"err":       err.Error(),
			"client_id": client.ClientId,
		}).Error("error connecting to the server")
		return
	}
	jReq, _ = json.Marshal(request)
	conn.Write(jReq)
	d := json.NewDecoder(conn)
	err = d.Decode(&resp)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("error connecting to the TCP socket to initiate the read, shutting down...")
		return
	}
	log.WithFields(log.Fields{
		"msg": resp,
	}).Debug("Received message from the server")
}
