package consensus

import (
	"NRBlockchain/common"
	"NRBlockchain/lamport"
	"context"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/manifoldco/promptui"
)

var (
	Client      *BlockchainClient
	GlobalClock = 0
)

// TODO: See if the client id and the lamport clock pid can be made the same for now
type BlockchainClient struct {
	ClientId   int
	PortNumber int
	Clock      *lamport.LamportClock
	Q          []int
	Peers      []int
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
	}
}

// Start essentially does the following -
// 1. Register the rest of the clients with the current client
// 2. Establishes a connection to each of the client
// 3. Logs the essential information needed
func (client *BlockchainClient) Start(ctx context.Context) {
	client.registerClients(ctx)
	go client.startTransactions(ctx)
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

/*validate := func(input string) error {
	_, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return errors.New("Invalid number")
	}
	return nil
}*/

// startTransactions keeps a connection alive in order to receive any events from the rest of the clients
func (client *BlockchainClient) startTransactions(ctx context.Context) {
	var (
		err error
		receiverClient, amountStr string
		amount float64
		transactionType string
		receiverClientId int
		txn string
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
			txn = common.BalanceTxn
			client.sendRequest(ctx, &common.ServerRequest{
				TxnType:  txn,
				ClientId: client.ClientId,
			})
		case "Transfer":
			txn = common.TransferTxn
			prompt := promptui.Prompt{
				Label:    "Receiver Client",
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
				Label:     "Amount to be transacted",
				Default:   "",
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

func (client *BlockchainClient) sendRequest(ctx context.Context, request *common.ServerRequest) {

}