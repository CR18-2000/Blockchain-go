package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/common-nighthawk/go-figure"
	"github.com/ethereum/go-ethereum/common"
)

type myContract struct {
	name    string
	abi     string
	address common.Address
}

func main() {
	//clear screen
	exec.Command("clear")

	myFigure := figure.NewColorFigure("ETH-DEMO", "", "blue", true)
	myFigure.Print()

	l := log.New(os.Stdout, "", 0)
	l.Println()
	l.Println()
	l2 := log.New(os.Stdout, "[SC-Deployer] ", 0)
	l.Println("  Blockchain project demo --- Cereser Lorenzo/Regazzoni Cristina ")
	l.Println()
	l.Println()
	//contracts, err := deployContracts()
	l2.Println("[SC-Deployer] Starting contract deployment")
	contracts, err := deployMockContracts()
	if err != nil {
		l2.Fatalf("could not deploy contracts: %v", err)
	}
	l2.Println("All contracts deployed successfully.")
	var wg sync.WaitGroup
	quitChan := make(chan struct{})

	// Start all functions as goroutines
	wg.Add(7)
	go bankHandler(quitChan, &wg, contracts)
	go addressAuthHandler(quitChan, &wg, contracts)
	go TEE(quitChan, &wg, contracts)
	go DH1Handler(quitChan, &wg, contracts)
	go DH2Handler(quitChan, &wg, contracts)
	go DH3Handler(quitChan, &wg, contracts)
	go webserver(quitChan, &wg, contracts)

	inputReader := bufio.NewReader(os.Stdin)
	for {
		input, _ := inputReader.ReadString('\n')
		if strings.TrimSpace(input) == "q" {
			break
		}
	}

	fmt.Println("Stopping all daemons...")
	close(quitChan) // Signal all goroutines to stop
	wg.Wait()       // Wait for all goroutines to finish
	fmt.Println("All functions stopped. Demo finished.")

}

func deployContracts() (map[string]myContract, error) {

	var contracts = make(map[string]myContract)

	// Execute the script to deploy contracts
	cmd := exec.Command("./../../../../blockchain-smart-contracts/script.sh")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not run sc deploy command: %v", err)
	}
	log.Println("", string(out))

	// Process the output
	lines := strings.Split(string(out), "\n")
	var filteredLines []string
	for _, line := range lines {
		if strings.Contains(line, ">>>") {
			filteredLines = append(filteredLines, line)
		}
	}

	// Parse the output and populate the contracts map
	for _, line := range filteredLines {

		parts := strings.Split(line, ">>>")
		_name := strings.Split(parts[1], " ")[0]
		_address := strings.Split(parts[2], " ")[0]

		_abi, err := GetContractABI(_address)
		//_abi, err := GetContractABIlocal(_name)
		if err != nil {
			return nil, fmt.Errorf("error getting contract ABI for %s: %v", _name, err)
		}

		contracts[_name] = myContract{
			name:    _name,
			abi:     _abi,
			address: common.HexToAddress(_address),
		}
	}
	callSCMethod(contracts["CSC"], "[SC-Deployer][CSC] ", "changeBankAddress", contracts["BankSC"].address)
	callSCMethod(contracts["CSC"], "[SC-Deployer][CSC] ", "changeAddressAuthAddress", contracts["AddressAuth"].address)
	callSCMethod(contracts["CSC"], "[SC-Deployer][CSC] ", "changeDH1Address", contracts["DH1"].address)
	callSCMethod(contracts["CSC"], "[SC-Deployer][CSC] ", "changeDH2Address", contracts["DH2"].address)
	callSCMethod(contracts["CSC"], "[SC-Deployer][CSC] ", "changeDH3Address", contracts["DH3"].address)

	callSCMethod(contracts["BankSC"], "[SC-Deployer][BankSC] ", "changeCSCAddress", contracts["CSC"].address)

	callSCMethod(contracts["AddressAuth"], "[SC-Deployer][AddressAuth] ", "changeCSCAddress", contracts["CSC"].address)

	return contracts, nil
}

func deployMockContracts() (map[string]myContract, error) {
	var contracts = make(map[string]myContract)
	contracts["CSC"] = myContract{
		name:    "CSC",
		abi:     `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"address[]","name":"","type":"address[]"}],"name":"generateHMMKeysEvent","type":"event"},{"inputs":[],"name":"addressAuthAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address[]","name":"addressList","type":"address[]"}],"name":"addressReceiver","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"bankAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"_addressAuthAddress","type":"address"}],"name":"changeAddressAuthAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_bankAddress","type":"address"}],"name":"changeBankAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_dh1Address","type":"address"}],"name":"changeDH1Address","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_dh2Address","type":"address"}],"name":"changeDH2Address","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_dh3Address","type":"address"}],"name":"changeDH3Address","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_userAddress","type":"address"}],"name":"changeUserAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"dh1Address","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"dh2Address","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"dh3Address","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint8","name":"typeOfLoan","type":"uint8"},{"internalType":"string[]","name":"requirements","type":"string[]"}],"name":"dhSearch","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address[]","name":"addressList","type":"address[]"}],"name":"generateHMMKeys","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint8","name":"typeOfLoan","type":"uint8"}],"name":"quoteRequest","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"},{"internalType":"bytes","name":"party","type":"bytes"}],"name":"sendKeysToBank","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"},{"internalType":"address[]","name":"addressList","type":"address[]"}],"name":"sendKeysToDHs","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"},{"internalType":"bytes","name":"party","type":"bytes"}],"name":"sendKeysToUser","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"},{"internalType":"bytes","name":"bankParty","type":"bytes"},{"internalType":"bytes","name":"userParty","type":"bytes"},{"internalType":"address[]","name":"addressList","type":"address[]"}],"name":"uploadHMMKeys","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"userAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}]`,
		address: common.HexToAddress("0x77e6Dc32590Bd387Fa3AE8823a27232f4ac3c9b7"),
	}
	contracts["BankSC"] = myContract{
		name:    "BankSC",
		abi:     `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"uint8","name":"typeOfLoan","type":"uint8"}],"name":"getRequirementsEvent","type":"event"},{"inputs":[],"name":"CSCAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"_CSCAddress","type":"address"}],"name":"changeCSCAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint8","name":"_typeOfLoan","type":"uint8"}],"name":"requirementsRequest","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"typeOfLoan","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint8","name":"_typeOfLoan","type":"uint8"},{"internalType":"string[]","name":"requirements","type":"string[]"}],"name":"uploadRequirements","outputs":[],"stateMutability":"nonpayable","type":"function"}]`,
		address: common.HexToAddress("0xBF6030848A5D872A6622Fe9F3E7965EC2C415C68"),
	}
	contracts["AddressAuth"] = myContract{
		name:    "AddressAuth",
		abi:     `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"string[]","name":"","type":"string[]"}],"name":"addressRequestEvent","type":"event"},{"inputs":[],"name":"CSCAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"string[]","name":"requirements","type":"string[]"}],"name":"addressRequest","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address[]","name":"addresses","type":"address[]"}],"name":"addressUpload","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_CSCAddress","type":"address"}],"name":"changeCSCAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"_owner","type":"address"}],"name":"changeOwner","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}]`,
		address: common.HexToAddress("0x5d82cc8544D5C42964f337e56B81C044C1C876B6"),
	}

	contracts["DH1"] = myContract{
		name:    "DH1",
		abi:     `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"bytes","name":"param","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"pk","type":"bytes"}],"name":"receiveKeysEvent","type":"event"},{"inputs":[],"name":"CSCAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"_CSCAddress","type":"address"}],"name":"changeCSCAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"}],"name":"receiveKeys","outputs":[],"stateMutability":"nonpayable","type":"function"}]`,
		address: common.HexToAddress("0x5D47774b2121355FC13Aa5A0A5b1981E246c53Ec"),
	}

	contracts["DH2"] = myContract{
		name:    "DH2",
		abi:     `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"bytes","name":"param","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"pk","type":"bytes"}],"name":"receiveKeysEvent","type":"event"},{"inputs":[],"name":"CSCAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"_CSCAddress","type":"address"}],"name":"changeCSCAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"}],"name":"receiveKeys","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"version","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"}]`,
		address: common.HexToAddress("0x4C3882874698F054205c0a56f598991A75dC1866"),
	}

	contracts["DH3"] = myContract{
		name:    "DH3",
		abi:     `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"internalType":"bytes","name":"param","type":"bytes"},{"indexed":false,"internalType":"bytes","name":"pk","type":"bytes"}],"name":"receiveKeysEvent","type":"event"},{"inputs":[],"name":"CSCAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"_CSCAddress","type":"address"}],"name":"changeCSCAddress","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"owner","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"bytes","name":"param","type":"bytes"},{"internalType":"bytes","name":"pk","type":"bytes"}],"name":"receiveKeys","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"version","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"}]`,
		address: common.HexToAddress("0x1290FD9aF782DC3D3Ca2DD7A95dbb80Af6661471"),
	}
	return contracts, nil
}

func webserver(quitChan chan struct{}, wg *sync.WaitGroup, contracts map[string]myContract) {
	defer wg.Done()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
			<html>
			<head>
				<style>
					body {
						font-family: Arial, sans-serif;
						font-size: 25px;
						display: flex;
						flex-direction: column;
						align-items: center;
						justify-content: center;
						height: 100vh;
						margin: 0;
						background-color: #000; /* Black background */
						color: #fff; /* White text */
					}
					.loan-container {
						display: flex;
						justify-content: center;
						margin-bottom: 20px;
					}
					.loan {
						background-color: #8a2be2; /* Blue-violet background */
						padding: 15px;
						margin: 10px;
						border-radius: 5px;
						width: 300px;
						transition: background-color 0.3s;
						cursor: pointer;
						box-shadow: 0 4px 8px rgba(0,0,0,0.5);
					}
					.loan:hover {
						background-color: #7b68ee; /* Darker blue-violet on hover */
					}
					.loan.selected {
						border: 2px solid #fff; /* White border for selected loan */
					}
					.button {
						padding: 10px 20px;
						background-color: #8a2be2; /* Blue-violet button */
						color: white;
						border: none;
						border-radius: 5px;
						cursor: pointer;
						font-size: 16px;
						transition: background-color 0.3s;
					}
					.button:hover {
						background-color: #7b68ee; /* Darker blue-violet on hover */
					}
					#message {
						margin-top: 20px;
						color: #fff;
					}
				</style>
				<script>
					function selectLoan(loan) {
						document.querySelectorAll('.loan').forEach(function(div) {
							div.classList.remove('selected');
						});
						document.getElementById(loan).classList.add('selected');
					}

					function submitLoan() {
						var selectedLoan = document.querySelector('.loan.selected');
						if (selectedLoan) {
							var xhr = new XMLHttpRequest();
							xhr.open('GET', '/quote?loan=' + selectedLoan.id, true);
							xhr.onreadystatechange = function() {
								if (xhr.readyState == 4 && xhr.status == 200) {
									document.getElementById('message').innerText = xhr.responseText;
								}
							};
							xhr.send();
						} else {
							alert('Please select a loan type');
						}
					}
				</script>
			</head>
			<body>
				<div class="loan-container">
					<div id="1" class="loan" onclick="selectLoan('1')">
						<strong>Loan1</strong>
						<ul style="list-style-type: dotted;">
							<li>Salary </li>
							<li>Credit Score</li>
							<li>Debts</li>
						</ul>
					</div>
					<div id="2" class="loan" onclick="selectLoan('2')">
						<strong>Loan2</strong>
						<ul style="list-style-type: dotted;">
							<li>Credit Score</li>
							
						</ul>
					</div>
					<div id="3" class="loan" onclick="selectLoan('3')">
						<strong>Loan3</strong>
						<ul style="list-style-type: dotted;">
							<li>Salary</li>
							<li>Debt</li>
						</ul>
					</div>
				</div>
				<button class="button" onclick="submitLoan()">Submit Selected Loan</button>
				<div id="message"></div>
			</body>
			</html>
		`)
	})

	http.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		loan := r.URL.Query().Get("loan")
		loanID, err := strconv.ParseUint(loan, 10, 8)
		if err != nil {
			log.Printf("Error converting loan to uint8: %v", err)
			fmt.Fprintf(w, "Error occurred: %v", err)
			return
		}

		// Call smart contract method here
		// Replace "methodName" with your actual method name and provide necessary arguments
		if _, err := callSCMethod(contracts["CSC"], "[CSC] ", "quoteRequest", uint8(loanID)); err != nil {
			log.Printf("Error calling smart contract method: %v", err)
			fmt.Fprintf(w, "Error occurred: %v", err)
			return
		}

		fmt.Fprintf(w, "Smart contract method called successfully.")
		quitChan <- struct{}{}
	})

	server := &http.Server{Addr: ":8080"}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server ListenAndServe: %v", err)
		}
	}()

	<-quitChan // Wait for signal to quit
	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("HTTP server Shutdown: %v", err)
	}
}
