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

type AddressRequestEventData struct {
	Requirements []string
}

func addressAuthHandler(quitChan chan struct{}, wg *sync.WaitGroup, contracts map[string]myContract) {
	logger := log.New(os.Stdout, "[AddAuth daemon] : ", 0)
	defer wg.Done()

	// Connect to the Ethereum client
	client, err := ethclient.Dial(InfuraWSS)
	if err != nil {
		logger.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Instantiate the contract ABI
	contractAddress := common.HexToAddress(contracts["AddressAuth"].address.Hex())
	contractABI, err := abi.JSON(strings.NewReader(contracts["AddressAuth"].abi))
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
			case "addressRequestEvent":
				var eventData AddressRequestEventData

				// Unpacking the data from the log into the struct
				err := contractABI.UnpackIntoInterface(&eventData, "addressRequestEvent", vLog.Data)

				if err != nil {
					logger.Printf("Error unpacking event: %s", err.Error())
					continue
				}
				//get the requirements array from the event

				logger.Printf("Unpacked Requirements: %v", eventData.Requirements)

				addresses := []string{}
				//cycle through the requirements and append the coverted addresses to the array
				for i := 0; i < len(eventData.Requirements); i++ {
					switch eventData.Requirements[i] {
					case "SALARY":
						addresses = append(addresses, contracts["DH1"].address.Hex())
					case "DEBTS":
						addresses = append(addresses, contracts["DH2"].address.Hex())
					case "CREDITSCORE":
						addresses = append(addresses, contracts["DH3"].address.Hex())
					}
				}

				logger.Printf("Addresses: %s", addresses)

				//TODO: change with data from event
				callSCMethod(contracts["AddressAuth"], "[AddressAuth daemon] : ", "addressUpload", addresses)
			}
		case <-quitChan:
			// Exit the function
			logger.Println("quitting")
			return
		}
	}

}
