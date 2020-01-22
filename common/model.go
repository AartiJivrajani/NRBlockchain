package common

import "NonReplicated-Blockchain/lamport"

const (
	TransferTxn  = "Transfer"
	BalanceTxn   = "Balance"
	IncorrectTxn = "INCORRECT"
	ValidTxn     = "VALIDTXN"
	// Consensus Messages
	Request = "REQUEST"
	Ack     = "ACK"
	Release = "RELEASE"

	// server port
	ServerPort = 8003
)

var ClientPortMap = map[int]int{
	1: 8000,
	2: 8001,
	3: 8002,
}

type ServerRequest struct {
	// request type could be balance or transaction
	TxnType string `json:"TxnType"`
	// client which sent the request
	ClientId int     `json:"ClientId"`
	Amount   float64 `json:"Amount,omitempty"`
	Rcvr     int     `json:"Rcvr,omitempty"`
}

type ClientEvent struct {
}

type ConsensusEvent struct {
	Request      *ServerRequest        `json:"server_request,omitempty"`
	Message      string                `json:"message"`
	SourceClient int                   `json:"source_client"`
	DestClient   int                   `json:"dest_client"`
	CurrentClock *lamport.LamportClock `json:"clock"`
}

type ServerResponse struct {
	// the transaction type is checked first by the client, if could be either TransferTxn/BalanceTxn
	TxnType string `json:"TxnType"`
	// check if the transaction is valid. If not, set the Validity Response to "INCORRECT", else, VALIDTXN
	ValidityResp string `json:"ValidityResp"`
	// ClientId of the requester
	ClientId int `json:"ClientId"`
	// in case of balanceTxn, the balance is also returned
	BalanceAmt float64 `json:"BalanceAmt,omitempty"`
}
