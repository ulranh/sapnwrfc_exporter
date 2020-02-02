package internal

import (
	"math/rand"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

// ensure set/get of normal byte values is possible.
func Test_PwEncryptDecrypt(t *testing.T) {

	// create secret
	secretkey := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	_, err := rand.Read(secretkey)
	Ok(t, err)

	// encrypt
	pw := "123456"
	encrypted, err := PwEncrypt([]byte(pw), secretkey)
	Ok(t, err)

	// decrypt
	decrypted, err := PwDecrypt(encrypted, secretkey)
	Ok(t, err)

	Equals(t, decrypted, pw)
}

func Test_SecretKey(t *testing.T) {
	sk1, err := GetSecretKey()
	Ok(t, err)

	sk2, err := GetSecretKey()
	Ok(t, err)

	log.Println(len(sk2))
	Equals(t, len(sk1), 32)
	Equals(t, len(sk2), 32)
	Assert(t, string(sk1) != string(sk2), "values should not be equal")
}
