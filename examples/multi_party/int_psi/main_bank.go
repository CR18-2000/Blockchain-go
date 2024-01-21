package main

import (
	"log"
	"os"
	"time"

	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/heint"
)

func main_bank() {
	l := log.New(os.Stderr, "", 0)

	//set parameters
	params, pk, sk := setParamAndKeyBank()

	//get inputs
	input := setInputBank(params, a)

	//encrypt input
	encoder := heint.NewEncoder(params)
	encInputs := encPhaseBank(params, pk, encoder, input)

	//give it back to the other file
	sendInputBank(encInputs)

	//obtain back the result of evaluation
	encOut = getEncOutBank()

	//decrypt
	decryptionBank(params, encOut, sk, encoder)
	/*
		// Decrypt the result with the target secret key
		l.Println("> ResulPlaintextModulus:")
		decryptor := rlwe.NewDecryptor(params, tsk)
		ptres := heint.NewPlaintext(params, params.MaxLevel())
		elapsedDecParty := runTimed(func() {
			decryptor.Decrypt(encOut, ptres)
		})

		// Check the result
		res := make([]uint64, params.MaxSlots())
		if err := encoder.Decode(ptres, res); err != nil {
			panic(err)
		}*/
}

func setParamAndKeyBank(params heint.Parameters, pk *rlwe.PublicKey, sk *rlwe.SecretKey) (params heint.Parameters, pk *rlwe.PublicKey, sk *rlwe.SecretKey) {
	return params, pk, sk
}

func sendInputBank(encInputs []*rlwe.Ciphertext) (inputBank []*rlwe.Ciphertext) {
	inputBank = encInputs
	return
}

func getEncOutBank(out *rlwe.Ciphertext) (EncOut *rlwe.Ciphertext) {
	EncOut = out
	return
}

func encPhaseBank(params heint.Parameters, pk *rlwe.PublicKey, encoder *heint.Encoder, input []uint64) (encInputs []*rlwe.Ciphertext) {

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

func setInputBank(params heint.Parameters, a int) (expRes []uint64) {
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}
	expRes[len(expRes)-1] = uint64(a)
	return
}

func decryptionBank(params heint.Parameters, encOut *rlwe.Ciphertext, tsk *rlwe.SecretKey, encoder *heint.Encoder) (a int) {
	decryptor := rlwe.NewDecryptor(params, tsk)
	ptres := heint.NewPlaintext(params, params.MaxLevel())
	elapsedDecParty := runTimed(func() {
		decryptor.Decrypt(encOut, ptres)
	})
	res := make([]uint64, params.MaxSlots())
	if err := encoder.Decode(ptres, res); err != nil {
		panic(err)
	}
	a = int(res[len(res)-1])
	return
}
