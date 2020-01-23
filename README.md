# NonReplicated-Blockchain
CS270 Project-1: Non replicated blockchain

## Details of the project in the PDF [here](https://github.com/AartiJivrajani/NonReplicated-Blockchain/blob/master/CS271_Project1.pdf)

## Deployment Details

The project requirement was to assume 3 clients and a blockchain server. 
Feel free to poke around the project and run it too! 


Please follow the below steps for the same(PS: The project assumes that all the clients are running on localhost on different ports)

```bash
cd $GOPATH/NonReplicated-Blockchain/server
go run main.go
cd ..
cd client
# run the below commands on 3 different terminals
go run main.go --client_id=1
go run main.go --client_id=2
go run main.go --client_id=3
```