package blockchain

import (
	"container/list"
	"context"
)

var BlockChain *list.List

// Block represents a node in the linked list where the data is a
// Txn structure. There is also a pointer to the next node.
type Block struct {
	Txn  *Transaction
	Next *Block
}

type Transaction struct {
	Sender int
	Recvr  int
	Amount float64
}

// Initialize the blockchain
func BlockChainInit(ctx context.Context) {
	BlockChain = list.New()
}
