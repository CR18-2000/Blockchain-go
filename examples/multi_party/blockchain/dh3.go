package main

import (
	"context"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func DH3Handler(quitChan chan struct{}, wg *sync.WaitGroup, contracts map[string]myContract) {
	logger := log.New(os.Stdout, "[DH1 daemon] : ", 0)
	defer wg.Done()

	// Connect to the Ethereum client
	client, err := ethclient.Dial(InfuraWSS)
	if err != nil {
		logger.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Instantiate the contract ABI
	contractAddress := common.HexToAddress(contracts["DH3"].address.Hex())
	contractABI, err := abi.JSON(strings.NewReader(contracts["DH3"].abi))
	if err != nil {
		logger.Fatalf("Invalid ABI: %v", err)
	} else {
		logger.Println("ABI is valid")
	}

	// Subscribe to the contract events
	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddress},
		FromBlock: big.NewInt(-1), // "latest" block
	}

	eventCh := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), query, eventCh)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}

	for {
		select {
		case err := <-sub.Err():
			logger.Fatal(err)

		case vLog := <-eventCh:
			event, err := contractABI.EventByID(vLog.Topics[0])
			if err != nil {
				logger.Printf("Error getting event name: %s", err.Error())
				continue
			}
			logger.Printf("Received event: %s", event.Name)
			eventName := event.Name
			switch eventName {
			case "receiveKeysEvent":

				if err != nil {
					logger.Printf("Error unpacking event: %s", err.Error())
					continue
				}

				//get the requirements array from the event
				requirementsStr := event.Inputs[0].Type.String()
				requirements := strings.Split(requirementsStr, " ")
				logger.Printf("Requirements: %s", requirements)

				logger.Printf("Keys received")
			}
		case <-quitChan:
			// Exit the function
			logger.Println("quitting")
			return
		}
	}

}
