package database

import (
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func Read(db *badger.DB, key []byte) ([]byte, error) {
	var value []byte

	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)

		if err != nil {
			log.Panic(err)
		}

		value, err = item.ValueCopy(nil)

		if err != nil {
			log.Panic(err)
		}

		return nil
	})

	return value, err
}

func Update(db *badger.DB, key []byte, value []byte) error {
	err := db.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, value)

		if err != nil {
			log.Panic(err)
		}

		return nil
	})

	return err
}

func OpenDB(dir string) (*badger.DB, error) {
	opts := badger.DefaultOptions(dir)
	opts.Logger = nil

	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		return db, nil
	}
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
