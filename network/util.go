package network

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
)

func CmdToBytes(cmd string) []byte {
	var data [commandLength]byte

	for i, c := range cmd {
		data[i] = byte(c)
	}

	return data[:]
}

func BytesToCmd(bytes []byte) string {
	var cmd []byte

	for _, b := range bytes {
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}

	return fmt.Sprintf("%s", cmd)
}

func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}
