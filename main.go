package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type concurrentMap struct {
	sync.Mutex
	addresses map[string]bool
}

var path = "addresses"
var partitions = 7
var count int64
var oldCount int64
var addressesMap = concurrentMap { addresses: make(map[string]bool), }

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() - 1)

	count = int64(0)

	loadAddresses()

	value, _ := time.ParseDuration("5s")
	checkTimer := time.NewTimer(value)
	go func() {
		for {
			select {
			case <-checkTimer.C:
				log.Printf("Checked: %d, Speed: %d per second", count, count - oldCount)
				oldCount = count
				checkTimer.Reset(value)
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < partitions; i++ {
		wg.Add(1)
		addr := generateSeedAddress()
		log.Printf("Seed addr: %v\n", addr)
		go generateAddresses(addr)
	}
	wg.Wait()
}

func loadAddresses() {
	count := int64(0)

	// TODO: Maybe we pull this in from the ENV for kubes?
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {

		if strings.HasSuffix(f.Name(), "csv") {
			log.Printf("Processing csv: %s\n", f.Name())
			c, _ := os.Open(path + string(os.PathSeparator) + f.Name())
			r := csv.NewReader(bufio.NewReader(c))
			for {
				record, err := r.Read()
				if err == io.EOF {
					break
				}
				count++

				addressesMap.addresses[record[0]] = true
			}
			if err = c.Close(); err != nil {
				log.Fatal(err)
			}
		}

	}

	f, err := os.OpenFile(path + string(os.PathSeparator) + "balances.csv", os.O_CREATE | os.O_WRONLY, 0600)
	defer f.Close()

	if err != nil {
		log.Panic(err)
	}

	for k := range addressesMap.addresses {
		f.WriteString(k + "\n")
	}

	log.Printf("Number of addresses loaded: %d", count)
}

func generateSeedAddress() []byte {
	privKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privKey[i] = byte(rand.Intn(256))
	}
	return privKey
}

func generateAddresses(seedPrivKey []byte) {
	for ; ; {
		// Move backward through those bytes
		for i := 31; i > 0; i-- {
			if seedPrivKey[i] + 1 == 255 {
				seedPrivKey[i] = 0
			} else {
				seedPrivKey[i] += 1
				break
			}
		}

		// If this could be more optimized, this is where we'd get the most speed-up
		priv := crypto.ToECDSAUnsafe(seedPrivKey)
		address := crypto.PubkeyToAddress(priv.PublicKey)

		addressesMap.Lock()
		// Check to see if we have an address with a balance --
		if ok := addressesMap.addresses[strings.ToLower(address.Hex())]; ok {
			log.Printf("Found address with ETH balance, priv: %s, addr: %s", priv.D, address.Hex())
			writeToFound(fmt.Sprintf("Private: %s, Address: %s\n", priv.D, address.Hex()))
		}
		addressesMap.Unlock()
		count++
	}
}

func writeToFound(text string) {
	// TODO: Again ENV variable of where to store? Would like this to be kubes compat
	foundFileName := path + string(os.PathSeparator) + "found.txt"
	if _, err := os.Stat(foundFileName); os.IsNotExist(err) {
		_, _ = os.Create(foundFileName)
	}
	f, err := os.OpenFile(foundFileName, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	defer f.Close()
	if err != nil {
		log.Printf(err.Error())
	}
	_, err = f.WriteString(text)
	if err != nil {
		log.Printf(err.Error())
	}
}
