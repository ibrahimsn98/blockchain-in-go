package blockchain

import (
	"blockchain/main/database"
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
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
	Database *database.Database
}

type ChainIterator struct {
	CurrentHash []byte
	Database    *database.Database
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

	// Get database instance
	db, err := database.GetDatabase(path)
	Handle(err)

	// Create coinbase transaction
	cbtx := CoinbaseTx(address, genesisData)

	// Create genesis block
	genesis := Genesis(cbtx)
	fmt.Println("Genesis created")

	// Store genesis block data
	err = db.Update(genesis.Hash, genesis.Serialize())
	Handle(err)

	// Set last hash as genesis block hash
	err = db.Update([]byte("lh"), genesis.Hash)
	Handle(err)

	// Return chain that has only genesis block
	blockchain := BlockChain{genesis.Hash, db}
	return &blockchain
}

func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)

	if DBExists(path) == false {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	db, err := database.GetDatabase(path)
	Handle(err)

	// Get last block hash
	lastHash, err := db.Read([]byte("lh"))
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

	// Get last block hash
	lastHash, err := chain.Database.Read([]byte("lh"))
	Handle(err)

	// Get serialized last block data
	lastBlockBytes, err := chain.Database.Read(lastHash)
	Handle(err)

	// Deserialize byte data to block
	lastBlock := *Deserialize(lastBlockBytes)

	// Create new block
	newBlock := CreateBlock(transactions, lastHash, lastBlock.Height+1)

	// Store new block
	err = chain.Database.Update(newBlock.Hash, newBlock.Serialize())
	Handle(err)

	// Update last block hash
	err = chain.Database.Update([]byte("lh"), newBlock.Hash)
	Handle(err)

	chain.LastHash = newBlock.Hash

	return newBlock
}

func (chain *BlockChain) AddBlock(block *Block) {

	// If the chain already has this block, cancel the process
	_, err := chain.Database.Read(block.Hash)
	if err == nil {
		return
	}

	// Store new block
	err = chain.Database.Update(block.Hash, block.Serialize())
	Handle(err)

	// Get last block hash
	lastHash, err := chain.Database.Read([]byte("lh"))
	Handle(err)

	// Get serialized last block data
	lastBlockData, err := chain.Database.Read(lastHash)
	Handle(err)

	// Deserialize byte data to block
	lastBlock := Deserialize(lastBlockData)

	// If block height is bigger than last block height, set it as the last block
	if block.Height > lastBlock.Height {
		err := chain.Database.Update([]byte("lh"), block.Hash)
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

		// Iterate until the first block
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	blockData, err := chain.Database.Read(blockHash)

	if err != nil {
		log.Panic("block is not found")
	}

	return *Deserialize(blockData), err
}

func (chain *BlockChain) GetBestHeight() int {
	lastHash, err := chain.Database.Read([]byte("lh"))
	Handle(err)

	lastBlockData, err := chain.Database.Read(lastHash)
	Handle(err)

	lastBlock := *Deserialize(lastBlockData)

	return lastBlock.Height
}

func (chain *BlockChain) Iterator() *ChainIterator {
	return &ChainIterator{chain.LastHash, chain.Database}
}

func (iter *ChainIterator) Next() *Block {
	currentBlockData, err := iter.Database.Read(iter.CurrentHash)
	Handle(err)

	block := Deserialize(currentBlockData)
	iter.CurrentHash = block.PrevHash

	return block
}

// Finds unspent transaction outputs
// Unspent means that these outputs were not referenced in any inputs
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		// Iterate transactions
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			// Iterate transaction outputs
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

		// Iterate transactions
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			// Iterate transaction outputs
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

func (chain *BlockChain) SignTransaction(tx *Transaction, privateKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	// Iterate previous transactions
	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privateKey, prevTXs)
}

func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {

	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		fmt.Println("TX: " + hex.EncodeToString(tx.ID) + "\nInput Prev: " + hex.EncodeToString(in.ID))

		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)

		fmt.Println("Tx in DB: " + hex.EncodeToString(prevTX.ID))

		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}
