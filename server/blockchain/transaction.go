package blockchain

import (
	"NRBlockchain/common"
	"context"
)

func (server *BlockchainServer) GetBalance(ctx context.Context,
	requestMsg *common.ServerRequest) (float64, error) {

	var (
		balance float64
	)
	for block := server.BlockChain.Front(); block != nil; block = block.Next() {
		if block.Value.(Transaction).Recvr == requestMsg.ClientId {
			balance += block.Value.(Transaction).Amount
		}
		if block.Value.(Transaction).Sender == requestMsg.ClientId {
			balance -= block.Value.(Transaction).Amount
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
		//err error
	)
	// check if the transaction is valid
	balance, _ = server.GetBalance(ctx, requestMsg)
	if balance < requestMsg.Amount {
		return &common.ServerResponse{
			TxnType:      common.TransferTxn,
			ValidityResp: common.IncorrectTxn,
			ClientId:     requestMsg.ClientId,
		}, nil
	}
	// add the new transaction to the blockchain
	server.BlockChain.PushBack(&Transaction{
		Sender: requestMsg.ClientId,
		Recvr:  requestMsg.Rcvr,
		Amount: requestMsg.Amount,
	})
	// return success message
	return &common.ServerResponse{
		TxnType:      common.TransferTxn,
		ValidityResp: common.ValidTxn,
		ClientId:     requestMsg.ClientId,
	}, nil
}
