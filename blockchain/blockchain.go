package blockchain

import (
	"blockchain/main/database"
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
	"os"
	"runtime"
)

const (
	dbPath      = "/tmp/blocks_%s"
	genesisData = "First Transaction from Genesis"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type ChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func DBExists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}

	return true
}

func InitBlockChain(address, nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)

	if DBExists(path) {
		fmt.Println("Blockchain already exists.")
		runtime.Goexit()
	}

	db, err := database.OpenDB(path)
	Handle(err)

	cbtx := CoinbaseTx(address, genesisData)
	genesis := Genesis(cbtx)
	fmt.Println("Genesis created")

	err = database.Update(db, genesis.Hash, genesis.Serialize())
	Handle(err)

	err = database.Update(db, []byte("lh"), genesis.Hash)
	Handle(err)

	blockchain := BlockChain{genesis.Hash, db}
	return &blockchain
}

func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)

	if DBExists(path) == false {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	db, err := database.OpenDB(path)
	Handle(err)

	lastHash, err := database.Read(db, []byte("lh"))
	Handle(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
	for _, tx := range transactions {
		if chain.VerifyTransaction(tx) != true {
			log.Panic("Invalid Transaction")
		}
	}

	lastHash, err := database.Read(chain.Database, []byte("lh"))
	Handle(err)

	lastBlockData, err := database.Read(chain.Database, lastHash)
	Handle(err)

	lastBlock := *Deserialize(lastBlockData)

	newBlock := CreateBlock(transactions, lastHash, lastBlock.Height+1)

	err = database.Update(chain.Database, newBlock.Hash, newBlock.Serialize())
	Handle(err)

	err = database.Update(chain.Database, []byte("lh"), newBlock.Hash)
	Handle(err)

	chain.LastHash = newBlock.Hash

	return newBlock
}

func (chain *BlockChain) AddBlock(block *Block) {
	_, err := database.Read(chain.Database, block.Hash)

	if err == nil {
		return
	}

	err = database.Update(chain.Database, block.Hash, block.Serialize())
	Handle(err)

	lastHash, err := database.Read(chain.Database, []byte("lh"))
	Handle(err)

	lastBlockData, err := database.Read(chain.Database, lastHash)
	Handle(err)

	lastBlock := Deserialize(lastBlockData)

	if block.Height > lastBlock.Height {
		err := database.Update(chain.Database, []byte("lh"), block.Hash)
		Handle(err)

		chain.LastHash = block.Hash
	}
}

func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		blocks = append(blocks, block.Hash)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	blockData, err := database.Read(chain.Database, blockHash)

	if err != nil {
		log.Panic("block is not found")
	}

	return *Deserialize(blockData), err
}

func (chain *BlockChain) GetBestHeight() int {
	lastHash, err := database.Read(chain.Database, []byte("lh"))
	Handle(err)

	lastBlockData, err := database.Read(chain.Database, lastHash)
	Handle(err)

	lastBlock := *Deserialize(lastBlockData)

	return lastBlock.Height
}

func (chain *BlockChain) Iterator() *ChainIterator {
	iter := &ChainIterator{chain.LastHash, chain.Database}

	return iter
}

func (iter *ChainIterator) Next() *Block {
	currentBlockData, err := database.Read(iter.Database, iter.CurrentHash)
	Handle(err)

	block := Deserialize(currentBlockData)
	iter.CurrentHash = block.PrevHash

	return block
}

func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return UTXO
}

func (chain *BlockChain) FindUnspentTransactions(pubKeyHash []byte) []Transaction {
	var unspentTxs []Transaction

	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				if out.IsLockedWithKey(pubKeyHash) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					if in.UsesKey(pubKeyHash) {
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return unspentTxs
}

func (chain *BlockChain) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransactions(pubKeyHash)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs {
			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOuts
}

func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("transaction does not exist")
}

func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {

	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}
