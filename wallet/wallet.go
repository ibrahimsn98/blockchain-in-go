package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"log"

	"golang.org/x/crypto/ripemd160"
)

const (
	checksumLength = 4
	version        = byte(0x00)
)

type Wallet struct {
	PrivateKey ecdsa.PrivateKey
	PublicKey  []byte
}

func (w Wallet) Address() []byte {
	// Returns RIPEMD-160 hash
	pubHash := PublicKeyHash(w.PublicKey)

	// Add version byte in front of RIPEMD-160 hash
	versionedHash := append([]byte{version}, pubHash...)

	// Get 4 bytes checksum
	checksum := Checksum(versionedHash)

	// Add the 4 checksum bytes at the end of extended RIPEMD-160 hash
	// This is the 25-byte binary wallet address
	fullHash := append(versionedHash, checksum...)

	// Convert the result from a byte string into a base58 string
	address := Base58Encode(fullHash)
	return address
}

func ValidateAddress(address string) bool {
	// Decode Base58 address string
	pubKeyHash := Base58Decode([]byte(address))

	// Remove checksum bytes
	actualChecksum := pubKeyHash[len(pubKeyHash)-checksumLength:]

	version := pubKeyHash[0]

	// Remove the version byte
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-checksumLength]

	// Get 4 bytes checksum
	targetChecksum := Checksum(append([]byte{version}, pubKeyHash...))

	// Compare actual and true checksum bytes
	return bytes.Compare(actualChecksum, targetChecksum) == 0
}

func NewKeyPair() (ecdsa.PrivateKey, []byte) {
	// Create an elliptic curve
	curve := elliptic.P256()

	// Generate private ECDSA key
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}

	// Take the corresponding public key generated with it
	pub := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)
	return *private, pub
}

func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	wallet := Wallet{private, public}

	return &wallet
}

func PublicKeyHash(pubKey []byte) []byte {
	// Perform SHA-256 hashing on the public key
	pubHash := sha256.Sum256(pubKey)

	// Perform RIPEMD-160 hashing on the result of SHA-256
	hasher := ripemd160.New()

	_, err := hasher.Write(pubHash[:])
	if err != nil {
		log.Panic(err)
	}

	publicRipEMD := hasher.Sum(nil)
	return publicRipEMD
}

func Checksum(payload []byte) []byte {
	// Perform SHA-256 hash on the extended RIPEMD-160 hash
	firstHash := sha256.Sum256(payload)

	// Perform SHA-256 hash on the result of the previous SHA-256 hash
	secondHash := sha256.Sum256(firstHash[:])

	// Take the first 4 bytes of the second SHA-256 hash. This is the address checksum
	return secondHash[:checksumLength]
}
