// Copyright © 2020 Ulrich Anhalt <ulrich.anhalt@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	crypt "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/crypto/nacl/secretbox"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ulranh/sapnwrfc_exporter/internal"

	"golang.org/x/crypto/ssh/terminal"
)

// pwCmd represents the pw command
var pwCmd = &cobra.Command{
	Use:   "pw",
	Short: "Set passwords for the systems in the config file",
	Long: `With the command pw you can set the passwords for the systems you want to monitor. You can set the password for one system or several systems separated by comma. For example:
	sapnwrfc_exporter pw --system d01
	sapnwrfc_exporter pw -s d01,d02 --config ./.sapnwrfc_exporter.toml`,
	Run: func(cmd *cobra.Command, args []string) {

		config, err := getConfig()
		if err != nil {
			exit("Can't handle config file: ", err)
		}

		// set timeout for pw system connection test
		config.Timeout = 5

		err = config.SetPw(cmd)
		if err != nil {
			exit("Can't set password: ", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(pwCmd)

	pwCmd.PersistentFlags().StringP("system", "s", "", "name(s) of system(s) separated by comma")
	pwCmd.MarkPersistentFlagRequired("system")

}

// SetPw - save password(s) of system(s) database user to the config file
func (config *Config) SetPw(cmd *cobra.Command) error {

	fmt.Print("Password: ")
	pw, err := terminal.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, "setPw(ReadPassword)")
	}

	systems, err := cmd.Flags().GetString("system")
	if err != nil {
		return errors.Wrap(err, "setPw(GetString)")
	}

	config.Secret, err = config.AddSecret(systems, pw)
	if err != nil {
		return errors.Wrap(err, "setPw(newSecret)")
	}

	viper.Set("secret", config.Secret)
	err = viper.WriteConfig()
	if err != nil {
		return errors.Wrap(err, "setPw(WriteConfig)")
	}

	// connection test for all systems
	secretMap, err := config.GetSecretMap()
	if err != nil {
		return errors.Wrap(err, "SetPw(GetSecretMap)")
	}
	for _, s := range config.Systems {
		pw, err := GetPassword(secretMap, s.Name)
		if err != nil {
			log.WithFields(log.Fields{
				"system": s.Name,
			}).Error("no password found for system")
		}
		conn, err := connect(s, pw)
		if err != nil {
			log.WithFields(log.Fields{
				"system": s.Name,
			}).Warn("no connection to system possible")
			continue
		}
		conn.Close()
	}

	return nil
}

// AddSecret - create encrypted secret for system(s)
func (config *Config) AddSecret(systems string, pw []byte) ([]byte, error) {
	var err error

	secret, err := config.GetSecretMap()
	if err != nil {
		return nil, errors.Wrap(err, "AddSecret(GetSecretMap)")
	}

	// create secret key once if it doesn't exist
	if _, ok := secret.Name["secretkey"]; !ok {

		secret.Name = make(map[string][]byte)
		secret.Name["secretkey"], err = GetSecretKey()
		if err != nil {
			return nil, errors.Wrap(err, "AddSecret(GetSecretKey)")
		}
	}

	// encrypt password
	encPw, err := PwEncrypt(pw, secret.Name["secretkey"])
	if err != nil {
		return nil, errors.Wrap(err, "AddSecret(PwEncrypt)")
	}

	for _, system := range strings.Split(systems, ",") {

		// check, if cmd line system exists in configfile
		sInfo := config.FindSystem(low(system))
		if "" == sInfo.Name {
			log.WithFields(log.Fields{
				"system": low(system),
			}).Error("missing system")
			return nil, errors.New("Did not find system in configfile system slice.")
		}

		// add password to secret map
		secret.Name[low(system)] = encPw
	}

	// write pw information back to the config file
	newSecret, err := proto.Marshal(&secret)
	if err != nil {
		return nil, errors.Wrap(err, "AddSecret(Marshal)")
	}

	return newSecret, nil
}

// FindSystem - check if cmpSystem already exists in configfile
func (config *Config) FindSystem(cmpSystem string) SystemInfo {
	for _, system := range config.Systems {
		if low(system.Name) == low(cmpSystem) {
			return system
		}
	}
	return SystemInfo{}
}

// GetSecretKey - create secret key once
func GetSecretKey() ([]byte, error) {

	key := make([]byte, 32)
	rand.Seed(time.Now().UnixNano())
	if _, err := rand.Read(key); err != nil {
		return nil, errors.Wrap(err, "GetSecretKey(rand.Read)")
	}

	return key, nil
}

// PwEncrypt - encrypt system password
func PwEncrypt(bytePw, byteSecret []byte) ([]byte, error) {

	var secretKey [32]byte
	copy(secretKey[:], byteSecret)

	var nonce [24]byte
	if _, err := io.ReadFull(crypt.Reader, nonce[:]); err != nil {
		return nil, errors.Wrap(err, "PwEncrypt(ReadFull)")
	}

	return secretbox.Seal(nonce[:], bytePw, &nonce, &secretKey), nil
}

// PwDecrypt - decrypt system password
func PwDecrypt(encrypted, byteSecret []byte) (string, error) {

	var secretKey [32]byte
	copy(secretKey[:], byteSecret)

	var decryptNonce [24]byte
	copy(decryptNonce[:], encrypted[:24])
	decrypted, ok := secretbox.Open(nil, encrypted[24:], &decryptNonce, &secretKey)
	if !ok {
		return "", errors.New("PwDecrypt(secretbox.Open)")
	}

	return string(decrypted), nil
}

// GetSecretMap - unmarshal secret bytes
func (config *Config) GetSecretMap() (internal.Secret, error) {

	if config.Secret == nil {
		return internal.Secret{}, nil
	}

	// unmarshal secret byte array
	var secret internal.Secret
	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		return internal.Secret{}, errors.Wrap(err, "GetSecretMap(Unmarshal)")
	}
	return secret, nil
}

// GetPassword - decrypt password
func GetPassword(secret internal.Secret, system string) (string, error) {

	// get encrypted system pw
	if _, ok := secret.Name[low(system)]; !ok {
		return "", errors.New("GetPassword(encrypted system pw info does not exist)")
	}

	// decrypt system password
	pw, err := PwDecrypt(secret.Name[low(system)], secret.Name["secretkey"])
	if err != nil {
		return "", errors.Wrap(err, "GetPassword(PwDecrypt)")
	}
	return pw, nil
}
