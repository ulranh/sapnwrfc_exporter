package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/sapnwrfc_exporter/internal"
	"golang.org/x/crypto/ssh/terminal"
)

// save password of system(s) sap user
func (config *Config) pw(flags map[string]*string) error {

	fmt.Print("Password: ")
	pw, err := terminal.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, " pw - ReadPassword")
	}

	// fill map with existing secrets from configfile
	var secret internal.Secret
	if err = proto.Unmarshal(config.Secret, &secret); err != nil {
		return errors.Wrap(err, " system  - Unmarshal")
	}

	// create secret key once if it doesn't exist
	if _, ok := secret.Name["secretkey"]; !ok {

		secret.Name = make(map[string][]byte)
		secret.Name["secretkey"], err = internal.GetSecretKey()
		if err != nil {
			return errors.Wrap(err, "passwd - getPassword")
		}
	}

	err = config.newSecret(secret, flags["system"], pw)
	if err != nil {
		return errors.Wrap(err, " pw - newSecret")
	}

	// marshal the secret map
	config.Secret, err = proto.Marshal(&secret)
	if err != nil {
		return errors.Wrap(err, " ImportSystems - Marshal")
	}

	// write changes to configfile
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(config); err != nil {
		return errors.Wrap(err, " ImportSystems - Marshal")
	}

	f, err := os.Create(*flags["config"])
	if err != nil {
		return errors.Wrap(err, "tomlExport - os.Create")
	}
	_, err = f.WriteString(buf.String())
	if err != nil {
		return errors.Wrap(err, "tomlExport - f.WriteString")
	}

	return nil
}

// add encrypted password to secret map if connect to system is o.k.
func (config *Config) newSecret(secret internal.Secret, systems *string, pw []byte) error {

	encPw, err := internal.PwEncrypt(pw, secret.Name["secretkey"])
	if err != nil {
		return errors.Wrap(err, "newSecret - PwEncrypt ")
	}

	sysMap := make(map[string]bool)
	for _, sys := range strings.Split(*systems, ",") {
		sysMap[strings.ToLower(sys)] = false
	}
	for _, sys := range config.Systems {
		sysName := strings.ToLower(sys.Name)

		// check if pw system exists in configfile system slice
		if _, ok := sysMap[sysName]; !ok {
			continue
		}
		sysMap[sysName] = true

		// connection test
		sys.password = string(pw)
		_, err := connect(sys, serverInfo{sys.Server, sys.Sysnr})
		if err != nil {
			continue
		}

		// add password to secret map
		secret.Name[sysName] = encPw
	}

	for k, v := range sysMap {
		if !v {
			log.WithFields(log.Fields{
				"system": k,
			}).Error("Did not find system in configfile systems slice.")
		}
	}

	return nil
}
