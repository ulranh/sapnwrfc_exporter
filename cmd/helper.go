package cmd

import (
	"fmt"
	"strings"

	"github.com/sap/gorfc/gorfc"
	log "github.com/sirupsen/logrus"
)

// establish connection to sap system
func connect(system systemInfo, password string) (*gorfc.Connection, error) {
	c, err := gorfc.ConnectionFromParams(
		gorfc.ConnectionParameters{
			"Dest":   system.Name,
			"User":   system.User,
			"Passwd": password,
			"Client": system.Client,
			"Lang":   system.Lang,
			"Ashost": system.Server,
			"Sysnr":  system.Sysnr,

			"Mshost": system.Mshost,
			"Msserv": system.Msserv,
			"Group":  system.Group,

			"Saprouter": system.Saprouter,
			// "Trace":     "1",
		},
	)
	if err != nil {
		log.WithFields(log.Fields{
			"system": system.Name,
			"server": system.Server,
			"error":  err,
		}).Error("Can't connect to system with user/password")
		return nil, err
	}

	return c, nil
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
