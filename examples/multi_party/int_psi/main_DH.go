package main

import (
	"log"
	"os"
	"time"

	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/heint"
)

func main_DataHolder() {
	l := log.New(os.Stderr, "", 0)

	//set parameters
	params, pk := setParamAndKeyDH()

	//get inputs
	input := setInputDH(params, a)

	//encrypt input
	encoder := heint.NewEncoder(params)
	encInputs := encPhaseDH(params, pk, encoder, input)

	//give it back to the other file
	sendInputDH(encInputs)

}

func setParamAndKeyDH(params heint.Parameters, pk *rlwe.PublicKey) (params heint.Parameters, pk *rlwe.PublicKey) {
	return params, pk
}

func sendInputDH(encInputs *rlwe.Ciphertext) (inputDH *rlwe.Ciphertext) {
	inputDH = encInputs
	return
}

func encPhaseDH(params heint.Parameters, pk *rlwe.PublicKey, encoder *heint.Encoder, input []uint64) (encInputs []*rlwe.Ciphertext) {

	l := log.New(os.Stderr, "", 0)

	//I think this encrypt the input of all parties, here we don't have parties
	/*encInputs = make([]*rlwe.Ciphertext, len(P))
	for i := range encInputs {
		encInputs[i] = heint.NewCiphertext(params, 1, params.MaxLevel())
	}*/

	// Each party encrypts its input vector
	l.Println("> Encrypt Phase")
	encryptor := rlwe.NewEncryptor(params, pk)

	pt := heint.NewPlaintext(params, params.MaxLevel())
	//elapsedEncryptParty = runTimedParty(func() {
	//for i, pi := range P {
	if err := encoder.Encode(input, pt); err != nil {
		panic(err)
	}
	if err := encryptor.Encrypt(pt, encInputs); err != nil {
		panic(err)
	}
	//}
	//})

	elapsedEncryptCloud = time.Duration(0)
	l.Printf("\tdone (cloud: %s, party: %s)\n", elapsedEncryptCloud, elapsedEncryptParty)

	return
}

func setInputDH(params heint.Parameters, a int) (expRes []uint64) {
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}
	expRes[len(expRes)-1] = uint64(a)
	return
}
