package main

import (
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/heint"
	"github.com/tuneinsight/lattigo/v5/mhe"
	"github.com/tuneinsight/lattigo/v5/ring"
	"github.com/tuneinsight/lattigo/v5/utils/sampling"
)

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
type multTask struct {
	wg              *sync.WaitGroup
	op1             *rlwe.Ciphertext
	opOut           *rlwe.Ciphertext
	res             *rlwe.Ciphertext
	elapsedmultTask time.Duration
}

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

func maincrypto() {

	l := log.New(os.Stderr, "", 0)

	l.Println("  Blockchain project demo --- Cereser Lorenzo/Regazzoni Cristina   ")

	// Largest for n=8192: 512 parties
	N := 2 // Default number of parties
	var err error
	if len(os.Args[1:]) >= 1 {
		N, err = strconv.Atoi(os.Args[1])
		check(err)
	}

	NGoRoutine := 1 // Default number of Go routines
	if len(os.Args[1:]) >= 2 {
		NGoRoutine, err = strconv.Atoi(os.Args[2])
		check(err)
	}

	// Creating encryption parameters from a default params with logN=14, logQP=438 with a plaintext modulus T=65537
	params, err := heint.NewParametersFromLiteral(heint.ParametersLiteral{
		LogN:             4,
		LogQ:             []int{56, 55, 55, 54, 54, 54},
		LogP:             []int{55, 55},
		PlaintextModulus: 65537,
	})
	if err != nil {
		panic(err)
	}

	crs, err := sampling.NewKeyedPRNG([]byte{'l', 'a', 't', 't', 'i', 'g', 'o'})
	if err != nil {
		panic(err)
	}

	encoder := heint.NewEncoder(params)

	// Create each party, and allocate the memory for all the shares that the protocols will need
	P := genparties(params, N)
	l.Println("[TEE]: parties and key pairs generated")

	// Inputs & expected result
	genInputs(params, P)
	l.Println("[Bank]: inputs [2, 3, 1]")
	l.Println("[DH1]: inputs [6]")
	l.Println("[DH2]: inputs [8]")
	l.Println("[DH3]: inputs [1]")

	// 1) Collective public key generation
	pk := ckgphase(params, crs, P)

	// 2) Collective relinearization key generation
	rlk := rkgphase(params, crs, P)

	evk := rlwe.NewMemEvaluationKeySet(rlk)

	encInputs := encPhase(params, P, pk, encoder)
	l.Println("[Bank]: inputs encrypted")
	l.Println("[DH1]: inputs encrypted")
	l.Println("[DH2]: inputs encrypted")
	l.Println("[DH3]: inputs encrypted")

	l.Println("[TEE]: evaluation phase")
	encRes := make([]*rlwe.Ciphertext, len(encInputs[0]))
	for i := 0; i < len(encInputs[0]); i++ {
		Inputs := make([]*rlwe.Ciphertext, len(P))
		Inputs[0] = encInputs[0][i]
		Inputs[1] = encInputs[1][i]
		encRes[i] = evalPhaseMul(params, NGoRoutine, Inputs, evk)
	}

	Inputs := make([]*rlwe.Ciphertext, len(P))
	Inputs[0] = encRes[0]
	Inputs[1] = encRes[1]
	result := evalPhaseAdd(params, NGoRoutine, Inputs, evk)
	for i := 2; i < len(encRes); i++ {
		Inputs[0] = result
		Inputs[1] = encRes[i]
		result = evalPhaseAdd(params, NGoRoutine, Inputs, evk)
	}

	l.Println("[TEE]: key switching phase using bank's public key")
	encOut := pcksPhase(params, P[0].pk, result, P)

	// Decrypt the result with the target secret key
	decryptor := rlwe.NewDecryptor(params, P[0].sk)
	ptres := heint.NewPlaintext(params, params.MaxLevel())
	decryptor.Decrypt(encOut, ptres)

	// Check the result
	res := make([]uint64, params.MaxSlots())
	if err := encoder.Decode(ptres, res); err != nil {
		panic(err)
	}

	l.Println("[Bank]: ciphertext decrypted using the secret key; result: ", int(res[len(res)-1]))

	l.Println("[TEE]: key switching phase using user's public key")
	encOut = pcksPhase(params, P[1].pk, result, P)

	// Decrypt the result with the target secret key
	decryptor = rlwe.NewDecryptor(params, P[1].sk)
	ptres = heint.NewPlaintext(params, params.MaxLevel())
	decryptor.Decrypt(encOut, ptres)

	// Check the result
	res = make([]uint64, params.MaxSlots())
	if err := encoder.Decode(ptres, res); err != nil {
		panic(err)
	}

	l.Println("[User]: ciphertext decrypted using the secret key; result: ", int(res[len(res)-1]))

}

func encPhase(params heint.Parameters, P []*party, pk *rlwe.PublicKey, encoder *heint.Encoder) (encInputs [][]*rlwe.Ciphertext) {

	//l := log.New(os.Stderr, "", 0)

	encInputs = make([][]*rlwe.Ciphertext, len(P))
	for i := range encInputs {
		encInputs[i] = make([]*rlwe.Ciphertext, len(P[i].input))
	}
	for i := range encInputs {
		for j := 0; j < len(P[i].input); j++ {
			encInputs[i][j] = heint.NewCiphertext(params, 1, params.MaxLevel())
		}
	}

	// Each party encrypts its input vector
	encryptor := rlwe.NewEncryptor(params, pk)

	pt := heint.NewPlaintext(params, params.MaxLevel())
	elapsedEncryptParty = runTimedParty(func() {
		for i, pi := range P {
			for j := 0; j < len(pi.input); j++ {
				if err := encoder.Encode(pi.input[j], pt); err != nil {
					panic(err)
				}
				if err := encryptor.Encrypt(pt, encInputs[i][j]); err != nil {
					panic(err)
				}
			}
		}
	}, len(P))

	elapsedEncryptCloud = time.Duration(0)
	return
}

func evalPhaseMul(params heint.Parameters, NGoRoutine int, encInputs []*rlwe.Ciphertext, evk rlwe.EvaluationKeySet) (encRes *rlwe.Ciphertext) {

	//l := log.New(os.Stderr, "", 0)

	encLvls := make([][]*rlwe.Ciphertext, 0)
	encLvls = append(encLvls, encInputs)
	for nLvl := len(encInputs) / 2; nLvl > 0; nLvl = nLvl >> 1 {
		encLvl := make([]*rlwe.Ciphertext, nLvl)
		for i := range encLvl {
			encLvl[i] = heint.NewCiphertext(params, 2, params.MaxLevel())
		}
		encLvls = append(encLvls, encLvl)
	}
	encRes = encLvls[len(encLvls)-1][0]

	evaluator := heint.NewEvaluator(params, evk)
	// Split the task among the Go routines
	tasks := make(chan *multTask)
	workers := &sync.WaitGroup{}
	workers.Add(NGoRoutine)
	//l.Println("> Spawning", NGoRoutine, "evaluator goroutine")
	for i := 1; i <= NGoRoutine; i++ {
		go func(i int) {
			evaluator := evaluator.ShallowCopy() // creates a shallow evaluator copy for this goroutine
			for task := range tasks {
				task.elapsedmultTask = runTimed(func() {
					// 1) Multiplication of two input vectors
					if err := evaluator.Mul(task.op1, task.opOut, task.res); err != nil {
						panic(err)
					}
					// 2) Relinearization
					if err := evaluator.Relinearize(task.res, task.res); err != nil {
						panic(err)
					}
				})
				task.wg.Done()
			}
			//l.Println("\t evaluator", i, "down")
			workers.Done()
		}(i)
		//l.Println("\t evaluator", i, "started")
	}

	// Start the tasks
	taskList := make([]*multTask, 0)
	elapsedEvalCloud = runTimed(func() {
		for i, lvl := range encLvls[:len(encLvls)-1] {
			nextLvl := encLvls[i+1]
			wg := &sync.WaitGroup{}
			wg.Add(len(nextLvl))
			for j, nextLvlCt := range nextLvl {
				task := multTask{wg, lvl[2*j], lvl[2*j+1], nextLvlCt, 0}
				taskList = append(taskList, &task)
				tasks <- &task
			}
			wg.Wait()
		}
	})
	elapsedEvalCloudCPU = time.Duration(0)
	for _, t := range taskList {
		elapsedEvalCloudCPU += t.elapsedmultTask
	}
	elapsedEvalParty = time.Duration(0)

	close(tasks)
	workers.Wait()

	return
}

func evalPhaseAdd(params heint.Parameters, NGoRoutine int, encInputs []*rlwe.Ciphertext, evk rlwe.EvaluationKeySet) (encRes *rlwe.Ciphertext) {

	//l := log.New(os.Stderr, "", 0)

	encLvls := make([][]*rlwe.Ciphertext, 0)
	encLvls = append(encLvls, encInputs)
	for nLvl := len(encInputs) / 2; nLvl > 0; nLvl = nLvl >> 1 {
		encLvl := make([]*rlwe.Ciphertext, nLvl)
		for i := range encLvl {
			encLvl[i] = heint.NewCiphertext(params, 2, params.MaxLevel())
		}
		encLvls = append(encLvls, encLvl)
	}
	encRes = encLvls[len(encLvls)-1][0]

	evaluator := heint.NewEvaluator(params, evk)
	// Split the task among the Go routines
	tasks := make(chan *multTask)
	workers := &sync.WaitGroup{}
	workers.Add(NGoRoutine)
	//l.Println("> Spawning", NGoRoutine, "evaluator goroutine")
	for i := 1; i <= NGoRoutine; i++ {
		go func(i int) {
			evaluator := evaluator.ShallowCopy() // creates a shallow evaluator copy for this goroutine
			for task := range tasks {
				task.elapsedmultTask = runTimed(func() {
					// 1) Multiplication of two input vectors
					if err := evaluator.Add(task.op1, task.opOut, task.res); err != nil {
						panic(err)
					}
					// 2) Relinearization
					if err := evaluator.Relinearize(task.res, task.res); err != nil {
						panic(err)
					}
				})
				task.wg.Done()
			}
			//l.Println("\t evaluator", i, "down")
			workers.Done()
		}(i)
		//l.Println("\t evaluator", i, "started")
	}

	// Start the tasks
	taskList := make([]*multTask, 0)
	elapsedEvalCloud = runTimed(func() {
		for i, lvl := range encLvls[:len(encLvls)-1] {
			nextLvl := encLvls[i+1]
			wg := &sync.WaitGroup{}
			wg.Add(len(nextLvl))
			for j, nextLvlCt := range nextLvl {
				task := multTask{wg, lvl[2*j], lvl[2*j+1], nextLvlCt, 0}
				taskList = append(taskList, &task)
				tasks <- &task
			}
			wg.Wait()
		}
	})
	elapsedEvalCloudCPU = time.Duration(0)
	for _, t := range taskList {
		elapsedEvalCloudCPU += t.elapsedmultTask
	}
	elapsedEvalParty = time.Duration(0)
	close(tasks)
	workers.Wait()

	return
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

func genInputs(params heint.Parameters, P []*party) {

	expRes := make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}

	expRes[len(expRes)-1] = uint64(6)
	P[1].input = append(P[1].input, expRes[:])
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}

	expRes[len(expRes)-1] = uint64(8)
	P[1].input = append(P[1].input, expRes[:])
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}

	expRes[len(expRes)-1] = uint64(1)
	P[1].input = append(P[1].input, expRes[:])
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}

	expRes[len(expRes)-1] = uint64(2)
	P[0].input = append(P[0].input, expRes[:])
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}

	expRes[len(expRes)-1] = uint64(3)
	P[0].input = append(P[0].input, expRes[:])
	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 0
	}

	expRes[len(expRes)-1] = uint64(1)
	P[0].input = append(P[0].input, expRes[:])

	return
}

func pcksPhase(params heint.Parameters, tpk *rlwe.PublicKey, encRes *rlwe.Ciphertext, P []*party) (encOut *rlwe.Ciphertext) {

	//l := log.New(os.Stderr, "", 0)

	// Collective key switching from the collective secret key to
	// the target public key

	pcks, err := mhe.NewPublicKeySwitchProtocol(params, ring.DiscreteGaussian{Sigma: 1 << 30, Bound: 6 * (1 << 30)})
	if err != nil {
		panic(err)
	}

	for _, pi := range P {
		pi.pcksShare = pcks.AllocateShare(params.MaxLevel())
	}

	elapsedPCKSParty = runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			pcks.GenShare(pi.sk, tpk, encRes, &pi.pcksShare)
		}
	}, len(P))

	pcksCombined := pcks.AllocateShare(params.MaxLevel())
	encOut = heint.NewCiphertext(params, 1, params.MaxLevel())
	elapsedPCKSCloud = runTimed(func() {
		for _, pi := range P {
			if err = pcks.AggregateShares(pi.pcksShare, pcksCombined, &pcksCombined); err != nil {
				panic(err)
			}
		}

		pcks.KeySwitch(encRes, pcksCombined, encOut)
	})

	return
}

func rkgphase(params heint.Parameters, crs sampling.PRNG, P []*party) *rlwe.RelinearizationKey {
	//l := log.New(os.Stderr, "", 0)

	rkg := mhe.NewRelinearizationKeyGenProtocol(params) // Relineariation key generation
	_, rkgCombined1, rkgCombined2 := rkg.AllocateShare()

	for _, pi := range P {
		pi.rlkEphemSk, pi.rkgShareOne, pi.rkgShareTwo = rkg.AllocateShare()
	}

	crp := rkg.SampleCRP(crs)

	elapsedRKGParty = runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.GenShareRoundOne(pi.sk, crp, pi.rlkEphemSk, &pi.rkgShareOne)
		}
	}, len(P))

	elapsedRKGCloud = runTimed(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.AggregateShares(pi.rkgShareOne, rkgCombined1, &rkgCombined1)
		}
	})

	elapsedRKGParty += runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.GenShareRoundTwo(pi.rlkEphemSk, pi.sk, rkgCombined1, &pi.rkgShareTwo)
		}
	}, len(P))

	rlk := rlwe.NewRelinearizationKey(params)
	elapsedRKGCloud += runTimed(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.AggregateShares(pi.rkgShareTwo, rkgCombined2, &rkgCombined2)
		}
		rkg.GenRelinearizationKey(rkgCombined1, rkgCombined2, rlk)
	})

	return rlk
}

func ckgphase(params heint.Parameters, crs sampling.PRNG, P []*party) *rlwe.PublicKey {

	//l := log.New(os.Stderr, "", 0)

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

	return pk
}
