package cmd_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/ulranh/sapnwrfc_exporter/cmd"
	"github.com/ulranh/sapnwrfc_exporter/internal"
)

var pw1 = "1234"
var pw2 = "5678"
var err error

func Test_AddSecret(t *testing.T) {
	assert := assert.New(t)

	// error GetSecretMap
	config := getTestConfig(0, 1)
	config.Secret = []byte{10, 50, 10, 3}
	_, err := config.AddSecret("d01", []byte(pw1))
	assert.NotNil(err)

	// error did not find system
	config = getTestConfig(0, 0)
	_, err = config.AddSecret("d01", []byte(pw1))
	assert.NotNil(err)

	// add first secret
	config = getTestConfig(1, 3)
	config.Secret, err = config.AddSecret("d01", []byte(pw1))
	assert.Nil(err)
	sm, err := config.GetSecretMap()
	assert.Nil(err)
	assert.Equal(len(sm.Name["secretkey"]), 32)
	pw, err := cmd.GetPassword(sm, "d01")
	assert.Nil(err)
	assert.Equal(pw, pw1)

	// add two more secrets
	config.Secret, err = config.AddSecret("d02,d03", []byte(pw2))
	assert.Nil(err)
	sm, err = config.GetSecretMap()
	assert.Nil(err)
	assert.Equal(len(sm.Name["secretkey"]), 32)
	pw, err = cmd.GetPassword(sm, "d02")
	assert.Nil(err)
	assert.Equal(pw, pw2)
	pw, err = cmd.GetPassword(sm, "d03")
	assert.Nil(err)
	assert.Equal(pw, pw2)
}

func Test_GetSecretMap(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(0, 1)

	// correct secret map
	secret, err := config.AddSecret("d01", []byte(pw1))
	assert.Nil(err)

	config.Secret = secret
	// sm, err := config.GetSecretMap()
	_, err = config.GetSecretMap()
	assert.Nil(err)
	// pw, err := cmd.GetPassword(sm, "d01")
	// assert.Equal(pw, pw1)

	// config.Secret == nil
	config.Secret = nil
	res, err := config.GetSecretMap()
	assert.Nil(err)
	assert.Equal(res, internal.Secret{})

	// bad secret
	config.Secret = []byte("no secret")
	_, err = config.GetSecretMap()
	assert.NotNil(err)
}

func Test_GetPassowrd(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(0, 1)

	// get pw for existing tenant
	config.Secret, err = config.AddSecret("d01", []byte(pw1))
	assert.Nil(err)
	sm, err := config.GetSecretMap()
	assert.Nil(err)

	// pw, err := cmd.GetPassword(sm, "d01")
	// assert.Nil(err)
	// assert.Equal(pw, pw1)

	// get pw for not existing tenant
	// pw, err = cmd.GetPassword(sm, "d09")
	// assert.NotNil(err)
	// assert.Equal(pw, "")

	// problem with pw decrypt
	sm.Name["secretkey"] = nil
	// pw, err = cmd.GetPassword(sm, "d01")
	// assert.NotNil(err)
	// assert.Equal(pw, "")
}

// ensure set/get of normal byte values is possible.
func Test_PwEncryptDecrypt(t *testing.T) {
	assert := assert.New(t)

	// create secret
	secretkey := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	_, err := rand.Read(secretkey)
	assert.Nil(err)

	// encrypt
	encrypted, err := cmd.PwEncrypt([]byte(pw1), secretkey)
	assert.Nil(err)

	// decrypt
	decrypted, err := cmd.PwDecrypt(encrypted, secretkey)
	assert.Nil(err)

	assert.Equal(decrypted, pw1)
}

func Test_SecretKey(t *testing.T) {
	assert := assert.New(t)

	sk1, err := cmd.GetSecretKey()
	assert.Nil(err)
	sk2, err := cmd.GetSecretKey()
	assert.Nil(err)

	assert.Equal(len(sk1), 32)
	assert.Equal(len(sk2), 32)
	assert.NotEqual(string(sk1), string(sk2))
}

func Test_FindTenant(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(2, 3)

	ti1 := config.FindSystem("d01")
	assert.Equal(ti1.Name, "d01")

	ti2 := config.FindSystem("D01")
	assert.Equal(ti2.Name, "d01")

	ti3 := config.FindSystem("D04")
	assert.Equal(ti3, cmd.SystemInfo{})
}

func Test_AddPwExistingTenantEmptySecret(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(2, 3)

	config.Secret, err = config.AddSecret("D01", []byte(pw1))
	assert.Nil(err)

	sm1, err := config.GetSecretMap()
	assert.Nil(err)

	tpw1, err := cmd.GetPassword(sm1, "d01")
	assert.Nil(err)
	assert.Equal(tpw1, pw1)
}

func Test_AddPwExistingTenants(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(2, 3)

	config.Secret, err = config.AddSecret("D01", []byte(pw1))
	assert.Nil(err)

	config.Secret, err = config.AddSecret("d03,D02", []byte(pw1))
	assert.Nil(err)

	config.Secret, err = config.AddSecret("D01", []byte(pw2))
	assert.Nil(err)

	sm, err := config.GetSecretMap()
	assert.Nil(err)

	tpw1, err := cmd.GetPassword(sm, "d03")
	assert.Nil(err)
	assert.Equal(tpw1, pw1)
	tpw2, err := cmd.GetPassword(sm, "d02")
	assert.Nil(err)
	assert.Equal(tpw2, pw1)

	tpw3, err := cmd.GetPassword(sm, "d01")
	assert.Nil(err)
	assert.Equal(tpw3, pw2)
}

func Test_AddPwNotExistingTenant(t *testing.T) {
	assert := assert.New(t)
	config := getTestConfig(2, 3)

	config.Secret, err = config.AddSecret("D04", []byte(pw1))
	assert.NotNil(err)
}
