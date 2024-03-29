package main

import (
	"log"
	"os"

	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/heint"
)

func main_user() {
	l := log.New(os.Stderr, "", 0)

	//set parameters
	params, sk := setParamAndKeyUser()

	//obtain back the result of evaluation
	encOut = getEncOutUser()

	encoder := heint.NewEncoder(params)

	//decrypt
	decryptionUser(params, encOut, sk, encoder)
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

func setParamAndKeyUser(params heint.Parameters, sk *rlwe.SecretKey) (params heint.Parameters, pk *rlwe.PublicKey, sk *rlwe.SecretKey) {
	return params, sk
}

func getEncOutUser(out *rlwe.Ciphertext) (EncOut *rlwe.Ciphertext) {
	EncOut = out
	return
}

func decryptionUser(params heint.Parameters, encOut *rlwe.Ciphertext, tsk *rlwe.SecretKey, encoder *heint.Encoder) (a int) {
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
