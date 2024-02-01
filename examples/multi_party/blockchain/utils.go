package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	Dh1Address = "0x2DAA3977F4fbC0Db5cfEc4DB13E85AE526c90598"
	//dh1 contract abi
	Dh1ABI = ``
	//dh2 contract address
	Dh2Address = "0x2DAA3977F4fbC0Db5cfEc4DB13E85AE526c90598"
	//dh2 contract abi
	Dh2ABI = ``
	//dh3 contract address
	Dh3Address = "0x2DAA3977F4fbC0Db5cfEc4DB13E85AE526c90598"
	//dh3 contract abi
	Dh3ABI = ``
	// infura websocket address
	InfuraWSS = "wss://sepolia.infura.io/ws/v3/45caed101fc844fd9339247c4358fc4c"
	//InfuraWSS = "ws://localhost:8545"
	// PK for account with test ether
	TestNetPK = "0x57c13b160ac131B0c38Be43Fd3c871a3D3E1248f"
	TestNetSK = "c4c834f65a9d3be8ac408da83f9c83d96fd6107f40ce8efa780ac8db9bb281c6"

	TestNetPKLocal  = "0x57c13b160ac131B0c38Be43Fd3c871a3D3E1248f"
	TestNetSKLocal  = "c4c834f65a9d3be8ac408da83f9c83d96fd6107f40ce8efa780ac8db9bb281c6"
	etherscanAPIKey = "RQ2CIT1QDYVCRA7SVU1UDJKDK64JZ8FDBU"

	// Etherscan API base URL
	etherscanAPIBaseURL = "http://api-sepolia.etherscan.io/api"
	sepoliaChainID      = 11155111
	hardhatChainID      = 31337
)

type ContractABI struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result"`
}

type ContractABI2 struct {
	ABI json.RawMessage `json:"abi"`
}

type myContract struct {
	name    string
	address common.Address
	abi     string
}

func GetContractABIlocal(contractName string) (string, error) {
	var basePath string = "./../../../../blockchain-smart-contracts/artifacts/contracts/"
	f, err := os.Open(fmt.Sprintf("%s%s.sol/%s.json", basePath, contractName, contractName))
	if err != nil {
		return "", err
	}
	defer f.Close()

	abiBytes, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	var abiResponse ContractABI2
	err = json.Unmarshal(abiBytes, &abiResponse)
	if err != nil {
		return "", err
	}

	return string(abiResponse.ABI), nil
}

// GetContractABI retrieves the ABI for a given contract address
func GetContractABI(contractAddress string) (string, error) {
	url := fmt.Sprintf("%s?module=contract&action=getabi&address=%s&apikey=%s", etherscanAPIBaseURL, contractAddress, etherscanAPIKey)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var abiResponse ContractABI
	err = json.Unmarshal(body, &abiResponse)
	if err != nil {
		return "", err
	}

	if abiResponse.Status != "1" {
		return "", fmt.Errorf("error retrieving ABI: %s", abiResponse.Message)
	}

	//save the abi to a file with the contract address as the name
	f, err := os.Create(fmt.Sprintf("%s.abi", contractAddress))
	if err != nil {
		return "", err
	}
	defer f.Close()

	exec.Command("abigen", "--abi", fmt.Sprintf("%s.abi", contractAddress), "--pkg", "main", "--out", fmt.Sprintf("%s.go", contractAddress)).Run()

	return abiResponse.Result, nil
}

func callSCMethod(contract myContract, loggerString string, method string, args ...interface{}) ([]byte, error) {
	// Connect to the Ethereum client

	logger := log.New(os.Stdout, loggerString, 0)
	logger.Printf("Calling method '%s(...)' on contract [%s]", method, contract.name)
	client, err := ethclient.Dial(InfuraWSS)
	if err != nil {
		logger.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Instantiate the contract ABI

	parsedABI, err := abi.JSON(strings.NewReader(contract.abi))
	if err != nil {
		return nil, err
	}

	// Pack the arguments using the contract ABI
	packedData, err := parsedABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}

	privateKey, err := crypto.HexToECDSA(TestNetSK)

	if err != nil {
		logger.Fatal(err)
	}

	nonce, err := client.PendingNonceAt(context.Background(), common.HexToAddress(TestNetPK))
	if err != nil {
		logger.Fatal(err)
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	// multiply by 2 to make sure the transaction goes through
	gasPrice.Mul(gasPrice, big.NewInt(2))
	if err != nil {
		logger.Fatal(err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(sepoliaChainID))
	if err != nil {
		logger.Fatal(err)
	}

	tx := types.NewTransaction(nonce, contract.address, big.NewInt(0), 200000, gasPrice, packedData)

	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		logger.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		logger.Fatal(err)
	}
	//wait for the transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), client, signedTx)
	if err != nil {
		logger.Fatal(err)
	}

	//get the transaction receipt
	logger.Printf("tx sent: %s", receipt.TxHash.Hex())

	//call the function on the samrt contract to upload the requirements
	return nil, nil
}
