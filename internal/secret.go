package internal

import (
	crypt "crypto/rand"
	"io"
	"math/rand"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/nacl/secretbox"
)

// GetSecretKey - create secret key once
func GetSecretKey() ([]byte, error) {

	key := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	if _, err := rand.Read(key); err != nil {
		return nil, errors.Wrap(err, "passwd - getPassword")
	}

	return key, nil
}

// PwEncrypt - encrypt tenant password
func PwEncrypt(bytePw, byteSecret []byte) ([]byte, error) {

	var secretKey [32]byte
	copy(secretKey[:], byteSecret)

	var nonce [24]byte
	if _, err := io.ReadFull(crypt.Reader, nonce[:]); err != nil {
		return nil, errors.Wrap(err, "secret - ReadFull")
	}

	return secretbox.Seal(nonce[:], bytePw, &nonce, &secretKey), nil

	// return encrypted, nil
}

// PwDecrypt - decrypt tenant password
func PwDecrypt(encrypted, byteSecret []byte) (string, error) {

	var secretKey [32]byte
	copy(secretKey[:], byteSecret)

	var decryptNonce [24]byte
	copy(decryptNonce[:], encrypted[:24])
	decrypted, ok := secretbox.Open(nil, encrypted[24:], &decryptNonce, &secretKey)
	if !ok {
		return "", errors.New("secret - decryption ")
	}

	return string(decrypted), nil
}
