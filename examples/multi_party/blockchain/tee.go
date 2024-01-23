package main

import (
	"context"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DmitriyVTitov/size"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/heint"
	"github.com/tuneinsight/lattigo/v5/mhe"
	"github.com/tuneinsight/lattigo/v5/utils/sampling"
)

var elapsedEncryptParty time.Duration
var elapsedEncryptCloud time.Duration
var elapsedCKGCloud time.Duration
var elapsedCKGParty time.Duration
var elapsedRKGCloud time.Duration
var elapsedRKGParty time.Duration
var elapsedPCKSCloud time.Duration
var elapsedPCKSParty time.Duration
var elapsedEvalCloudCPU time.Duration
var elapsedEvalCloud time.Duration
var elapsedEvalParty time.Duration

type GenerateHMMKeysEventData struct {
	Addresses []string
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func runTimed(f func()) time.Duration {
	start := time.Now()
	f()
	return time.Since(start)
}

func runTimedParty(f func(), N int) time.Duration {
	start := time.Now()
	f()
	return time.Duration(time.Since(start).Nanoseconds() / int64(N))
}

type party struct {
	sk         *rlwe.SecretKey
	pk         *rlwe.PublicKey
	rlkEphemSk *rlwe.SecretKey

	ckgShare    mhe.PublicKeyGenShare
	rkgShareOne mhe.RelinearizationKeyGenShare
	rkgShareTwo mhe.RelinearizationKeyGenShare
	pcksShare   mhe.PublicKeySwitchShare

	input [][]uint64
}

func TEE(quitChan chan struct{}, wg *sync.WaitGroup, contracts map[string]myContract) {

	logger := log.New(os.Stderr, "[TEE] ", 0)
	defer wg.Done()

	N := 2 // Default number of parties
	var err error
	if len(os.Args[1:]) >= 1 {
		N, err = strconv.Atoi(os.Args[1])
		check(err)
	}

	params, err := heint.NewParametersFromLiteral(heint.ParametersLiteral{
		LogN:             14,
		LogQ:             []int{56, 55, 55, 54, 54, 54},
		LogP:             []int{55, 55},
		PlaintextModulus: 65537,
	})
	check(err)

	crs, err := sampling.NewKeyedPRNG([]byte{'s', 'a', 't', 'o', 's', 'h', 'i', 'n', 'a', 'k', 'a', 'm', 'o', 't', 'o'})
	check(err)

	P := genparties(params, N)
	pk := ckgphase(params, crs, P)
	logger.Printf("KeyGen done (cloud: %s, party: %s)\n", elapsedCKGCloud, elapsedCKGParty)

	//serializing the public key

	client, err := ethclient.Dial(InfuraWSS)
	if err != nil {
		logger.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Instantiate the contract ABI
	contractAddress := common.HexToAddress(contracts["CSC"].address.Hex())
	contractABI, err := abi.JSON(strings.NewReader(contracts["CSC"].abi))
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
			case "generateHMMKeysEvent":
				var eventData GenerateHMMKeysEventData

				// Unpack the data from the vLog into an array of addresses
				err := contractABI.UnpackIntoInterface(&eventData, "generateHMMKeysEvent", vLog.Data)

				if err != nil {
					logger.Printf("Error unpacking event: %s", err.Error())
					continue
				}
				addresses := []string{}
				for i := 0; i < len(eventData.Addresses); i++ {
					addresses = append(addresses, eventData.Addresses[i])
				}

				logger.Printf("Serilizing and sending keys to bank")
				paramsBytes, err := params.MarshalBinary()
				check(err)

				pkBytes, err := pk.MarshalBinary()
				check(err)

				PZeroBytes, err := P[0].sk.MarshalBinary()
				check(err)

				POneBytes, err := P[1].sk.MarshalBinary()
				check(err)
				// print size of variables using unsafe.Sizeof()
				logger.Println("Size of paramsBytes: ", size.Of(paramsBytes))
				logger.Println("Size of pkBytes: ", size.Of(pkBytes))
				logger.Println("Size of PZeroBytes: ", size.Of(PZeroBytes))
				logger.Println("Size of POneBytes: ", size.Of(POneBytes))
				logger.Println("Size of addresses: ", size.Of(addresses))

				logger.Println("Calling uploadHMMKeys")
				callSCMethod(contracts["CSC"], "[TEE daemon] : ", "uploadHMMKeys", paramsBytes, pkBytes, PZeroBytes, POneBytes, addresses)

			}
		case <-quitChan:
			// Exit the function
			logger.Println("quitting")
			return
		}
	}

}

func genparties(params heint.Parameters, N int) []*party {

	// Create each party, and allocate the memory for all the shares that the protocols will need
	P := make([]*party, N)
	for i := range P {
		pi := &party{}
		pi.sk, pi.pk = rlwe.NewKeyGenerator(params).GenKeyPairNew()
		P[i] = pi
	}

	return P
}

func ckgphase(params heint.Parameters, crs sampling.PRNG, P []*party) *rlwe.PublicKey {

	l := log.New(os.Stderr, "[TEE]", 0)

	l.Println("> PublicKeyGen Phase")

	ckg := mhe.NewPublicKeyGenProtocol(params) // Public key generation
	ckgCombined := ckg.AllocateShare()
	for _, pi := range P {
		pi.ckgShare = ckg.AllocateShare()
	}

	crp := ckg.SampleCRP(crs)

	elapsedCKGParty = runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			ckg.GenShare(pi.sk, crp, &pi.ckgShare)
		}
	}, len(P))

	pk := rlwe.NewPublicKey(params)

	elapsedCKGCloud = runTimed(func() {
		for _, pi := range P {
			ckg.AggregateShares(pi.ckgShare, ckgCombined, &ckgCombined)
		}
		ckg.GenPublicKey(ckgCombined, crp, pk)
	})

	l.Printf("\tdone (cloud: %s, party: %s)\n", elapsedCKGCloud, elapsedCKGParty)

	return pk
}
