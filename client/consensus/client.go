package consensus

import (
	"NonReplicated-Blockchain/common"
	"NonReplicated-Blockchain/lamport"
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/manifoldco/promptui"

	"github.com/jpillora/backoff"

	log "github.com/Sirupsen/logrus"
)

var (
	Client              *BlockchainClient
	GlobalClock         = 0
	consensusMsgChan    = make(chan *common.ConsensusEvent)
	numMsg              = 0
	sendToServerChan    = make(chan bool)
	transactionStart    = make(chan bool)
	QLock               sync.Mutex
	clockLock           sync.Mutex
	showNextPrompt      = make(chan bool)
	allowHeadProcessing bool
)

type BlockchainClient struct {
	ClientId   int
	PortNumber int
	Clock      *lamport.LamportClock
	Q          *list.List

	Peers       []int
	PeerConnMap map[int]net.Conn
	index       int
}

func GetClient(ctx context.Context, clientId int, portNumber int) *BlockchainClient {
	return &BlockchainClient{
		Clock: &lamport.LamportClock{
			Timestamp: 0,
			PID:       0,
		},
		Q:           list.New(),
		ClientId:    clientId,
		PortNumber:  common.ClientPortMap[clientId],
		Peers:       make([]int, 2),
		PeerConnMap: make(map[int]net.Conn),
	}
}

func (client *BlockchainClient) handlePeerMessages(ctx context.Context, conn net.Conn) {
	var (
		err error
	)
	d := json.NewDecoder(conn)
	for {
		var resp *common.ConsensusEvent
		err = d.Decode(&resp)
		if err != nil {
			continue
		}
		consensusMsgChan <- resp
	}
}

func (client *BlockchainClient) handlePeerConnections(ctx context.Context, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
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
	// in case of client sending a message to the server, the consensus event will be nil
	clockLock.Lock()
	if msg == nil {
		GlobalClock += 1
	} else {
		msgClock := msg.CurrentClock.Timestamp
		if msgClock > GlobalClock {
			GlobalClock = msgClock + 1
		} else {
			GlobalClock += 1
		}
	}
	log.WithFields(log.Fields{
		"client_id": client.ClientId,
		"clock":     GlobalClock,
	}).Info("updated the clock")
	clockLock.Unlock()
}

func (client *BlockchainClient) AddToQ(ctx context.Context, msg *common.ConsensusEvent) {
	insertFront := true
	log.WithFields(log.Fields{
		"q": client.printRequestQ(ctx),
	}).Debug("before insertion")
	for el := client.Q.Back(); el != nil; el = el.Prev() {
		if msg.CurrentClock.Timestamp > el.Value.(*common.ConsensusEvent).CurrentClock.Timestamp {
			client.Q.InsertAfter(msg, el)
			insertFront = false
			break
		} else if msg.CurrentClock.Timestamp < el.Value.(*common.ConsensusEvent).CurrentClock.Timestamp {
			continue
		} else if msg.CurrentClock.Timestamp == el.Value.(*common.ConsensusEvent).CurrentClock.Timestamp {
			if msg.CurrentClock.PID < el.Value.(*common.ConsensusEvent).CurrentClock.PID {
				continue
			} else if msg.CurrentClock.PID > el.Value.(*common.ConsensusEvent).CurrentClock.PID {
				insertFront = false
				client.Q.InsertAfter(msg, el)
				break
			}
		}
	}
	if insertFront {
		client.Q.PushFront(msg)
	}
	log.WithFields(log.Fields{
		"q": client.printRequestQ(ctx),
	}).Debug("after insertion")
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
					numMsg = 0
					sendToServerChan <- true
					showNextPrompt <- true
					allowHeadProcessing = true
					continue
				} else {
					allowHeadProcessing = false
				}
			} else if msg.Message == common.Request {
				// process the request received from the other clients
				log.WithFields(log.Fields{
					"from_client_id": msg.SourceClient,
					"client_id":      client.ClientId,
				}).Debug("received request from peer")
				client.updateGlobalClock(ctx, msg)
				QLock.Lock()
				client.AddToQ(ctx, msg)
				QLock.Unlock()

				// send an ACK message to the requesting client.
				ackMsg := &common.ConsensusEvent{
					Request:      msg.Request,
					Message:      common.Ack,
					SourceClient: client.ClientId,
					DestClient:   msg.SourceClient,
					CurrentClock: &lamport.LamportClock{
						Timestamp: GlobalClock,
						PID:       client.ClientId,
					},
				}
				//sleep for sometime before sending back the ack
				time.Sleep(5 * time.Second)
				client.sendSingleMessage(ctx, client.PeerConnMap[ackMsg.DestClient], ackMsg)

			} else if msg.Message == common.Release {
				log.WithFields(log.Fields{
					"client_id":      client.ClientId,
					"from_client_id": msg.SourceClient,
					"list":           client.printRequestQ(ctx),
				}).Debug("Release message received")
				client.updateGlobalClock(ctx, msg)

				// remove the first element from the list
				QLock.Lock()
				e := client.Q.Front()
				if e != nil {
					client.Q.Remove(e)
				}
				QLock.Unlock()
				// once a release message is received, check the head of the queue,
				// if it is the same as the client, the client has the lock on the resource
				if client.Q.Front() != nil && client.Q.Front().Value.(*common.ConsensusEvent).SourceClient == client.ClientId {
					log.Debug("client found itself in front of the q... sending request to server")
					client.sendRequestToServer(ctx, client.Q.Front().Value.(*common.ConsensusEvent).Request, false)
				}
				log.WithFields(log.Fields{
					"list":      client.printRequestQ(ctx),
					"client_id": client.ClientId,
				}).Debug("after release request processing")
			}
		}
	}
}

// Start essentially does the following -
// 1. Register the rest of the clients with the current client
// 2. Establishes a connection to each of the client
// 3. Logs the essential information needed
func (client *BlockchainClient) Start(ctx context.Context) {
	client.registerClients(ctx)
	go client.startPeerListener(ctx)
	client.establishPeerConnections(ctx)
	// wait for the peer listener to start before we allow the transactions to begin
	go client.processClientMessages(ctx)
	go client.startTransactions(ctx)
}

// registerClients detects each peer that it has and stores its connection object
func (client *BlockchainClient) registerClients(ctx context.Context) {
	log.WithFields(log.Fields{
		"clientId":    client.ClientId,
		"clientClock": client.Clock.Timestamp,
	}).Debug("Registering the clients")

	if client.ClientId == 1 {
		client.Peers = []int{2, 3}
	} else if client.ClientId == 2 {
		client.Peers = []int{1, 3}
	} else if client.ClientId == 3 {
		client.Peers = []int{1, 2}
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
			os.Exit(0)

		case "Show Balance":
			txn = common.BalanceTxn
			client.sendRequestToServer(ctx, &common.ServerRequest{
				TxnType:  txn,
				ClientId: client.ClientId,
			}, true)
		case "Transfer":
			txn = common.TransferTxn
			prompt := promptui.Prompt{
				Label: "Receiver Client",
			}
			receiverClient, err = prompt.Run()
			if err != nil {
				log.WithFields(log.Fields{
					"error": err.Error(),
				}).Error("error fetching the client number from the command line")
				continue
			}
			receiverClientId, _ = strconv.Atoi(receiverClient)
			if receiverClientId == client.ClientId {
				log.Error("you cant send money to yourself!")
				continue
			}
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
			}, true)
		}
		<-showNextPrompt
	}
}

func (client *BlockchainClient) establishPeerConnections(ctx context.Context) {
	var (
		err  error
		conn net.Conn
		d    time.Duration
		b    = &backoff.Backoff{
			Min:    10 * time.Second,
			Max:    1 * time.Minute,
			Factor: 2,
			Jitter: true,
		}
	)

	log.WithFields(log.Fields{
		"client_id": client.ClientId,
		"peer_list": client.Peers,
	}).Debug("establishing peer connections with other clients")
	for _, peer := range client.Peers {
		PORT := ":" + strconv.Itoa(common.ClientPortMap[peer])
		d = b.Duration()
		for {
			conn, err = net.Dial("tcp", PORT)
			if err != nil {
				log.WithFields(log.Fields{
					"error":          err.Error(),
					"client_id":      client.ClientId,
					"peer_client_id": peer,
				}).Error("error connecting to the client")
				// if the connection fails, try to connect 3 times, post which just exit.
				if b.Attempt() <= 3 {
					time.Sleep(d)
					continue
				} else {
					log.Panic("Unable to connect to the peers")
				}
			} else {
				log.WithFields(log.Fields{
					"client_id":      client.ClientId,
					"peer_client_id": peer,
				}).Debug("Established connection with peer client")
				break
			}
		}
		client.PeerConnMap[peer] = conn
	}
}

// getConsensus sends a request to each client and waits for an ACK from each of them.
// once it receives an ACK from all the clients, it checks if it is at the head of the Priority Queue
// if yes, it makes a request to the blockchain server followed by multi-casting a release message
// to all the clients.
func (client *BlockchainClient) GetConsensus(ctx context.Context, request *common.ServerRequest) error {
	var (
		consensusReq *common.ConsensusEvent
		err          error
		cReq         []byte
	)
	consensusReq = &common.ConsensusEvent{
		Message:      common.Request,
		SourceClient: client.ClientId,
		DestClient:   0,
		CurrentClock: &lamport.LamportClock{
			Timestamp: GlobalClock,
			PID:       client.ClientId,
		},
		Request: request,
	}

	for _, peer := range client.Peers {
		if client.PeerConnMap[peer] == nil {
			log.WithFields(log.Fields{
				"client_id":      client.ClientId,
				"peer_client_id": peer,
			}).Panic("no pre-established connection found, exiting now...")
			return fmt.Errorf("no pre-established connection found")
		}
		log.WithFields(log.Fields{
			"from_client_id": client.ClientId,
			"to_client_id":   peer,
		}).Info("Sending Request to clients")

		consensusReq.DestClient = peer
		cReq, err = json.Marshal(consensusReq)
		_, err = client.PeerConnMap[peer].Write(cReq)
		if err != nil {
			log.WithFields(log.Fields{
				"from_client_id": client.ClientId,
				"to_client_id":   peer,
				"error":          err.Error(),
			}).Panic("error writing to the destination client")
		}

	}
	if err != nil {
		log.Error(err.Error())
		return fmt.Errorf(err.Error())
	}
	return nil
}

func (client *BlockchainClient) sendSingleMessage(ctx context.Context, conn net.Conn, msg *common.ConsensusEvent) {
	var (
		jMsg []byte
		err  error
		d    time.Duration
		b    = &backoff.Backoff{
			Min:    1 * time.Second,
			Max:    10 * time.Minute,
			Factor: 2,
			Jitter: true,
		}
	)
	log.WithFields(log.Fields{
		"from_client_id": client.ClientId,
		"to_client_id":   msg.DestClient,
		"msg":            msg,
	}).Debug("sending message to the peer")
	d = b.Duration()
	jMsg, _ = json.Marshal(msg)
	for {
		_, err = conn.Write(jMsg)
		if err != nil {
			log.WithFields(log.Fields{
				"error":          err.Error(),
				"from_client_id": client.ClientId,
				"to_client_id":   msg.DestClient,
				"msg":            msg,
			}).Error("error writing msg to the client socket")
			if b.Attempt() <= 3 {
				time.Sleep(d)
				continue
			} else {
				log.Panic("Unable to connect to the peers")
				break
			}
		} else {
			break
		}
	}
	client.updateGlobalClock(ctx, msg)
}

func (client *BlockchainClient) sendRequestToServer(ctx context.Context, request *common.ServerRequest, checkForLock bool) {
	var (
		err  error
		conn net.Conn
		jReq []byte
		resp *common.ServerResponse
	)
	if checkForLock {
		// push this client request to its own queue
		QLock.Lock()
		client.AddToQ(ctx, &common.ConsensusEvent{
			Request:      request,
			Message:      common.Request,
			SourceClient: client.ClientId,
			DestClient:   0,
			CurrentClock: &lamport.LamportClock{
				Timestamp: GlobalClock,
				PID:       client.ClientId,
			},
		},
		)
		QLock.Unlock()
		err = client.GetConsensus(ctx, request)
		if err != nil {
			log.Error("Consensus not reached")
			return
		}
		<-sendToServerChan
	}

	if client.Q.Front() != nil && client.Q.Front().Value.(*common.ConsensusEvent).SourceClient != client.ClientId {
		log.WithFields(log.Fields{
			"client_id": client.ClientId,
			"list":      client.printRequestQ(ctx),
		}).Debug("Received all ACKs, but critical section cant be accessed just yet.")
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
	client.updateGlobalClock(ctx, nil)
	jReq, _ = json.Marshal(request)
	log.Info("Sending request to server")
	conn.Write(jReq)
	d := json.NewDecoder(conn)
	err = d.Decode(&resp)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("error connecting to the TCP socket to initiate the read, shutting down...")
		return
	}
	if resp.TxnType == common.TransferTxn {
		log.Info("=============================================================================================")
		log.WithFields(log.Fields{
			"Transaction Status": resp.ValidityResp,
		}).Info("TRANSFER TRANSACTION VALIDITY FROM SERVER")
		log.Info("=============================================================================================")
	} else if resp.TxnType == common.BalanceTxn {
		log.Info("=============================================================================================")
		log.WithFields(log.Fields{
			"Balance": resp.BalanceAmt,
		}).Info("SERVER RESPONSE FOR BALANCE TRANSACTION")
		log.Info("=============================================================================================")
	}
	QLock.Lock()
	e := client.Q.Front()
	if e != nil {
		//log.WithFields(log.Fields{
		//	"q": client.printRequestQ(ctx),
		//}).Info("removing head from Q")
		client.Q.Remove(e)
		log.WithFields(log.Fields{
			"q": client.printRequestQ(ctx),
		}).Info("removed head from Q")
	}
	QLock.Unlock()
	client.sendReleaseMessage(ctx)
}

func (client *BlockchainClient) sendReleaseMessage(ctx context.Context) {
	for _, peer := range client.Peers {
		client.sendSingleMessage(ctx, client.PeerConnMap[peer], &common.ConsensusEvent{
			Request:      nil,
			Message:      common.Release,
			SourceClient: client.ClientId,
			DestClient:   peer,
			CurrentClock: &lamport.LamportClock{
				Timestamp: GlobalClock,
				PID:       client.ClientId,
			},
		})
	}
}

func (client *BlockchainClient) printRequestQ(ctx context.Context) string {
	var l string
	for block := client.Q.Front(); block != nil; block = block.Next() {
		l = l + strconv.Itoa(block.Value.(*common.ConsensusEvent).SourceClient) + "(" + strconv.Itoa(block.Value.(*common.ConsensusEvent).CurrentClock.Timestamp) + ")"
		if block.Next() != nil {
			l = l + "->"
		}
	}
	return l
}
