package cmd

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sap/gorfc/gorfc"
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/sapnwrfc_exporter/internal"
)

// establish connection to sap system
func connect(system *systemInfo, server serverInfo) (*gorfc.Connection, error) {
	c, err := gorfc.ConnectionFromParams(
		gorfc.ConnectionParameters{
			"Dest":   system.Name,
			"User":   system.User,
			"Passwd": system.password,
			"Client": system.Client,
			"Lang":   system.Lang,
			"Ashost": server.name,
			"Sysnr":  server.sysnr,
			// Ashost: config.Systems[s].Server,
			// Sysnr:  config.Systems[s].Sysnr,
			// Saprouter: "/H/203.13.155.17/S/3299/W/xjkb3d/H/172.19.137.194/H/",
		},
	)
	if err != nil {
		log.WithFields(log.Fields{
			"system": system.Name,
			"server": server.name,
			"error":  err,
		}).Error("Can't connect to system with user/password")
		return nil, err
	}

	return c, nil
}

// add passwords and system servers to config.Systems
func (config *Config) addSystemData() error {
	var secret internal.Secret

	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		log.Fatal("Secret Values don't exist or are corrupted")
		return errors.Wrap(err, " system  - Unmarshal")
	}

	for _, system := range config.Systems {

		// decrypt password and add it to system config
		if _, ok := secret.Name[low(system.Name)]; !ok {
			log.WithFields(log.Fields{
				"system": system.Name,
			}).Error("Can't find password for system")
			continue
		}
		pw, err := internal.PwDecrypt(secret.Name[low(system.Name)], secret.Name["secretkey"])
		if err != nil {
			log.WithFields(log.Fields{
				"system": system.Name,
			}).Error("Can't decrypt password for system")
			continue
		}
		system.password = pw

		// retrieve system servers and add them to the system config
		c, err := connect(system, serverInfo{system.Server, system.Sysnr})
		if err != nil {
			continue
		}
		defer c.Close()

		params := map[string]interface{}{}
		r, err := c.Call("TH_SERVER_LIST", params)
		if err != nil {
			log.WithFields(log.Fields{
				"system": system.Name,
				"error":  err,
			}).Error("Can't call fumo th_server_list")
			continue
		}

		for _, v := range r["LIST"].([]interface{}) {
			appl := v.(map[string]interface{})
			info := strings.Split(strings.TrimSpace(appl["NAME"].(string)), "_")
			system.servers = append(system.servers, serverInfo{
				// !!!!! evtl up() nur fuer name -> testen
				name:  strings.TrimSpace(info[0]),
				sysnr: strings.TrimSpace(info[2]),
			})
		}
	}
	return nil
}

// convert interface int values to string
func interface2String(namePart interface{}) string {

	switch val := namePart.(type) {
	case string:
		return val
	case int64, int32, int16, int8, int, uint64, uint32, uint8, uint:
		// return strconv.FormatInt(val, 10)
		return fmt.Sprint(val)
	default:
		return ""
	}
}

// true if every item in sublice exists in slice
func subSliceInSlice(subSlice []string, slice []string) bool {
	for _, vs := range subSlice {
		for _, v := range slice {
			if strings.EqualFold(vs, v) {
				goto nextCheck
			}
		}
		return false
	nextCheck:
	}
	return true
}

func up(str string) string {
	return strings.TrimSpace(strings.ToUpper(str))
}

func low(str string) string {
	return strings.TrimSpace(strings.ToLower(str))
}

func inFilter(line map[string]interface{}, filter map[string][]interface{}) bool {
	for field, values := range filter {
		for _, value := range values {
			if strings.EqualFold(interface2String(line[up(field)]), interface2String(value)) {
				return true
			}
		}
	}
	return false

}
