package database

import (
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Database struct {
	DB *badger.DB
}

func GetDatabase(dir string) (*Database, error) {
	opts := badger.DefaultOptions(dir)
	opts.Logger = nil

	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return &Database{db}, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		return &Database{db}, nil
	}
}

func (db *Database) Iterator(prefetchValues bool, fn func(*badger.Iterator) error) error {
	err := db.DB.View(func(txn *badger.Txn) error {

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = prefetchValues
		it := txn.NewIterator(opts)
		defer it.Close()

		return fn(it)
	})

	return err
}

func (db *Database) Read(key []byte) ([]byte, error) {
	var value []byte

	err := db.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)

		if err != nil {
			return err
		}

		value, err = item.ValueCopy(nil)
		return err
	})

	return value, err
}

func (db *Database) Update(key []byte, value []byte) error {

	fmt.Println("Add Key: " + string(key))

	err := db.DB.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})

	return err
}

func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
}
