package consensus

import (
	"NRBlockchain/common"
	"NRBlockchain/lamport"
	"context"
	"encoding/json"
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
)

// TODO: See if the client id and the lamport clock pid can be made the same for now
type BlockchainClient struct {
	ClientId   int
	PortNumber int
	Clock      *lamport.LamportClock
	Q          []int
	Peers      []int
	PeerConn   map[int]net.Conn
}

func GetClient(ctx context.Context, clientId int, portNumber int) *BlockchainClient {
	return &BlockchainClient{
		Clock: &lamport.LamportClock{
			Timestamp: 0,
			PID:       0,
		},
		Q:          make([]int, 3),
		ClientId:   clientId,
		PortNumber: portNumber,
		Peers:      make([]int, 2),
		PeerConn:   make(map[int]net.Conn),
	}
}

// Start essentially does the following -
// 1. Register the rest of the clients with the current client
// 2. Establishes a connection to each of the client
// 3. Logs the essential information needed
func (client *BlockchainClient) Start(ctx context.Context) {
	client.registerClients(ctx)
	go client.startTransactions(ctx)
	go client.GetConsensus(ctx)
}

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
			client.sendRequest(ctx, &common.ServerRequest{
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
			client.sendRequest(ctx, &common.ServerRequest{
				TxnType:  txn,
				ClientId: client.ClientId,
				Amount:   amount,
				Rcvr:     receiverClientId,
			})
		}
	}
}

func (client *BlockchainClient) consensusEventListener(ctx context.Context, listener net.Listener) {

	var (
		err  error
		conn net.Conn
		resp *common.ConsensusEvent
	)
	// write the event first
	defer listener.Close()
	for {
		conn, err = listener.Accept()
		if err != nil {

		}
		d := json.NewDecoder(conn)
		err = d.Decode(resp)
		if err != nil {

		}
		consensusMsgChan <- resp
	}
}

func (client *BlockchainClient) sendConsensusRequests(ctx context.Context) {
	var (
		consensusReq *common.ConsensusEvent
		err          error
		conn         net.Conn
		cReq         []byte
		listener     net.Listener
	)
	consensusReq = &common.ConsensusEvent{Message: common.Request}
	for peer := range client.Peers {
		PORT := ":" + strconv.Itoa(common.ClientPortMap[peer])
		listener, err = net.Listen("tcp", PORT)
		if err != nil {
			log.WithFields(log.Fields{
				"clientId":         client.ClientId,
				"connectingClient": peer,
			}).Panic("error connecting to the client, shutting down...")
			return
		}
		// send the consensus REQUEST message to this peer.
		conn, err = listener.Accept()
		if err != nil {

		}
		cReq, err = json.Marshal(consensusReq)
		_, err = conn.Write(cReq)
		if err != nil {

		}
		// for this client, wait for the ACKNOWLEDGEMENT response
		go client.consensusEventListener(ctx, listener)
	}

}

// getConsensus sends a request to each client and waits for an ACK from each of them.
// once it receives an ACK from all the clients, it checks if it is at the head of the Priority Queue
// if yes, it makes a request to the blockchain server followed by multi-casting a release message
// to all the clients.
func (client *BlockchainClient) GetConsensus(ctx context.Context) bool {
	go client.sendConsensusRequests(ctx)
	// open a connection to each of the clients.
	select {
	case <-consensusMsgChan:
		numMsg += 1
		if numMsg >= 2 {
			allDoneChan <- true
		}
	case <-allDoneChan:
		numMsg = 0
		return true
	}
	return false
}

func (client *BlockchainClient) sendRequest(ctx context.Context, request *common.ServerRequest) {
	var (
		err  error
		conn net.Conn
		jReq []byte
		resp *common.ServerResponse
	)
	log.Debug("in sendRequest method")
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

	//conn, err = listener.Accept()
	conn.Write(jReq)
	//_, _ = fmt.Fprintf(conn, string(jReq))
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
