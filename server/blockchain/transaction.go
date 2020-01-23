package blockchain

import (
	"NonReplicated-Blockchain/common"
	"context"
	"fmt"

	log "github.com/Sirupsen/logrus"
)

func (server *BlockchainServer) PrintBlockChain() {
	log.Info("=============================================================================================")
	log.Info("Current blockchain")
	for block := BlockChain.Front(); block != nil; block = block.Next() {
		fmt.Printf("Sender: %d,", block.Value.(*Transaction).Sender)
		fmt.Printf("Receiver: %d,", block.Value.(*Transaction).Recvr)
		fmt.Printf("Amount: %f", block.Value.(*Transaction).Amount)
		fmt.Printf("->")
	}
	fmt.Println("\n")
	log.Info("=============================================================================================")
}

func (server *BlockchainServer) GetBalance(ctx context.Context,
	requestMsg *common.ServerRequest) (float64, error) {

	var (
		balance float64
	)
	if BlockChain.Front() == nil {
		//log.Error("no blocks in the chain!")
		return float64(10), nil
	}
	balance = 10

	for block := BlockChain.Front(); block != nil; block = block.Next() {
		if block.Value.(*Transaction).Recvr == requestMsg.ClientId {
			balance += block.Value.(*Transaction).Amount
		}
		if block.Value.(*Transaction).Sender == requestMsg.ClientId {
			balance -= block.Value.(*Transaction).Amount
		}
	}
	return balance, nil
}

func (server *BlockchainServer) BalanceTransaction(ctx context.Context,
	requestMsg *common.ServerRequest) (*common.ServerResponse, error) {
	var balance float64
	balance, _ = server.GetBalance(ctx, requestMsg)
	return &common.ServerResponse{
		TxnType:      common.BalanceTxn,
		ValidityResp: common.ValidTxn,
		ClientId:     requestMsg.ClientId,
		BalanceAmt:   balance,
	}, nil
}

func (server *BlockchainServer) TransferTransaction(ctx context.Context,
	requestMsg *common.ServerRequest) (*common.ServerResponse, error) {
	var (
		balance float64
		resp    *common.ServerResponse
	)
	// check if the transaction is valid
	balance, _ = server.GetBalance(ctx, requestMsg)
	if balance < requestMsg.Amount {
		resp = &common.ServerResponse{
			TxnType:      common.TransferTxn,
			ValidityResp: common.IncorrectTxn,
			ClientId:     requestMsg.ClientId,
		}
	} else {
		resp = &common.ServerResponse{
			TxnType:      common.TransferTxn,
			ValidityResp: common.ValidTxn,
			ClientId:     requestMsg.ClientId,
		}
	}
	// add the new transaction to the blockchain
	BlockChain.PushBack(&Transaction{
		Sender: requestMsg.ClientId,
		Recvr:  requestMsg.Rcvr,
		Amount: requestMsg.Amount,
	})

	log.WithFields(log.Fields{
		"client_id": requestMsg.ClientId,
	}).Debug("Blockchain updated with transaction")

	server.PrintBlockChain()
	// return success message
	return resp, nil
}
