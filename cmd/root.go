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
	"fmt"
	"os"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sap/gorfc/gorfc"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// server information
type serverInfo struct {
	name string
	conn *gorfc.Connection
}

// system information
type SystemInfo struct {
	Name   string
	Usage  string
	Tags   []string
	User   string
	Lang   string
	Client string
	Server string
	Sysnr  string

	Mshost string
	Msserv string
	Group  string

	Saprouter string
}

// standard metric info
type tomlMetric struct {
	Name           string
	Help           string
	MetricType     string
	TagFilter      []string
	AllServers     bool
	FunctionModule string
	Params         map[string]interface{}
	TableData      TableInfo
	FieldData      FieldInfo
	StructureData  StructureInfo
}

// specific table metric info
type TableInfo struct {
	Table     string
	RowCount  map[string][]interface{}
	RowFilter map[string][]interface{}
}

// specific field metric info
type FieldInfo struct {
	FieldLabels []string
	FieldValues []string
}

// specific structure metric info
type StructureInfo struct {
	ExportStructure string
	StructureFields []string
}

type metricInfo struct {
	Name           string
	Help           string
	MetricType     string
	TagFilter      []string
	AllServers     bool
	FunctionModule string
	Params         map[string]interface{}
	special        dataReceiver
}

// interface for different handling of table- and field metrics
type dataReceiver interface {
	checkSpecialData() bool
	metricData(rawData map[string]interface{}, system SystemInfo, srvName string) []metricRecord
}

// config information for the whole process
type Config struct {
	Secret     []byte
	Systems    []SystemInfo // system info from toml file
	Metrics    []tomlMetric // metric info from toml file
	IntMetrics []metricInfo // adapted internal metrics
	passwords  map[string]string
	Timeout    uint
	port       string
}

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sapnwrfc_exporter",
	Short: "The purpose of this sapnwrfc_exporter is to support monitoring SAP ABAP instances with Prometheus and Grafana.",
	Long:  `The purpose of the sapnwrfc_exporter is to support monitoring SAP ABAP instances with Prometheus and Grafana. It is possible to count the occurrence for some defined values of a field in a SAP function module result table - for example the number of dialog, batch and update processes or the number of the SAP lock entries at a given time. Another possibility is to use the export field results of a function module as prometheus label values - for example to record the database client version and kernel patch level of the SAP instance. Also values of sap rfc function module result structures or fields can be monitored`,

	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		exit("RootCmd can't be executed: ", err)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.hana_sql_exporter.toml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			exit("Homedir can't be found: ", err)
		}

		// Search config in home directory with name ".sapnwrfc_exporter" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("toml")
		viper.SetConfigName(".sapnwrfc_exporter.toml")
	}
}

// read and unmarshal configfile into Config struct
func getConfig() (*Config, error) {
	var config Config

	// read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, errors.Wrap(err, "getConfig(ReadInConfig)")
	}

	// unmarshal config file
	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "getConfig(Unmarshal)")
	}

	return &config, nil
}

// check configfile
func (config *Config) checkConfig() error {

	// check if config secret exists
	if 0 == len(config.Secret) {
		return errors.New("the secret info is missing. Please add the passwords with \"sapnwrfc_exporter pw --system <system>\"")
	}

	err := config.fillInternalMetrics()
	if err != nil {
		return errors.Wrap(err, "getConfig(fillInternalMetrics)")
	}

	// check toml file systems
	err = config.checkTomlSystems()
	if err != nil {
		return errors.Wrap(err, "getConfig(checkTomlSystems)")
	}
	return nil
}

// create internal metrics struct slice from toml file
func (config *Config) fillInternalMetrics() error {
	for _, tm := range config.Metrics {
		mi, err := checkTomlMetric(tm)
		if err != nil {
			return errors.Wrap(err, "getConfig(checkTomlMetric)")
		}
		config.IntMetrics = append(config.IntMetrics, mi)
	}
	return nil
}

// check toml file metric entry and if ok return internal metric entry
func checkTomlMetric(tm tomlMetric) (metricInfo, error) {

	// check mandatory input
	if 0 == len(tm.Name) || 0 == len(tm.Help) || 0 == len(tm.MetricType) || 0 == len(tm.FunctionModule) {
		log.WithFields(log.Fields{
			"name":            tm.Name,
			"help":            tm.Help,
			"metric type":     tm.MetricType,
			"function module": tm.FunctionModule,
		}).Error("missing mandatory metric field(s)")
		return metricInfo{}, errors.New("checkTomlMetric(mandatory fields)")
	}
	if !strings.EqualFold(tm.MetricType, "counter") && !strings.EqualFold(tm.MetricType, "gauge") {
		log.WithFields(log.Fields{
			"name":        tm.Name,
			"metric type": tm.MetricType,
		}).Error("MetricType must be counter or gauge")
		return metricInfo{}, errors.New("checkTomlMetric(wrong metric type)")
	}

	// adapt tag filter
	var tfLow []string
	for _, tf := range tm.TagFilter {
		tfLow = append(tfLow, low(tf))
	}

	var data []dataReceiver
	for _, d := range []dataReceiver{&tm.FieldData, &tm.TableData, &tm.StructureData} {
		if d.checkSpecialData() {
			data = append(data, d)
		}
	}

	if len(data) == 0 {
		return metricInfo{}, errors.New("checkTomlMetric(" + tm.Name + " missing or wrong special info - field,structure or table)")
	}
	if len(data) > 1 {
		return metricInfo{}, errors.New("checkTomlMetric(" + tm.Name + " more than one special info - field,structure or table)")
	}

	return metricInfo{
		Name:           low(tm.Name),
		Help:           low(tm.Help),
		MetricType:     low(tm.MetricType),
		TagFilter:      tfLow,
		AllServers:     tm.AllServers,
		FunctionModule: up(tm.FunctionModule),
		Params:         tm.Params,
		special:        data[0],
	}, nil

}

// check toml metric field data
func (fi *FieldInfo) checkSpecialData() bool {
	if 0 == len(fi.FieldValues) && 0 == len(fi.FieldLabels) {
		return false
	}

	if len(fi.FieldValues) > 0 && len(fi.FieldLabels) > 0 {
		log.WithFields(log.Fields{
			"field values": fi.FieldValues,
			"field labels": fi.FieldLabels,
		}).Error("Fieldinfo: only one entry FieldLabels or FieldValues is allowed")
		return false
	}

	if len(fi.FieldLabels) > 0 || len(fi.FieldValues) > 0 {
		for i := range fi.FieldLabels {
			fi.FieldLabels[i] = low(fi.FieldLabels[i])
		}
		for i := range fi.FieldValues {
			fi.FieldValues[i] = low(fi.FieldValues[i])
		}
		return true
	}
	return false
}

// check toml metric structure data
func (si *StructureInfo) checkSpecialData() bool {

	if 0 == len(si.ExportStructure) && 0 == len(si.StructureFields) {
		return false
	}

	if len(si.ExportStructure) > 0 && len(si.StructureFields) > 0 {
		si.ExportStructure = up(si.ExportStructure)
		for i := range si.StructureFields {
			si.StructureFields[i] = low(si.StructureFields[i])
		}
		return true
	}

	log.WithFields(log.Fields{
		"ExportStructure": si.ExportStructure,
		"StructureFields": si.StructureFields,
	}).Error("StructureInfo: one or both entries missing")
	return false
}

// check toml metric table data
func (ti *TableInfo) checkSpecialData() bool {
	if 0 == len(ti.Table) && 0 == len(ti.RowCount) && 0 == len(ti.RowFilter) {
		return false
	}

	if len(ti.Table) > 0 && len(ti.RowCount) > 0 {
		ti.Table = up(ti.Table)
		return true
	}

	log.WithFields(log.Fields{
		"Table":    ti.Table,
		"RowCount": ti.RowCount,
	}).Error("TableInfo: one or both entries missing")
	return false
}

// check toml metric systems data
func (config *Config) checkTomlSystems() error {
	for i := range config.Systems {

		if 0 == len(config.Systems[i].Name) || 0 == len(config.Systems[i].Usage) || 0 == len(config.Systems[i].User) || 0 == len(config.Systems[i].Lang) || 0 == len(config.Systems[i].Client) || 0 == len(config.Systems[i].Server) || 0 == len(config.Systems[i].Sysnr) {
			log.WithFields(log.Fields{
				"name":   config.Systems[i].Name,
				"usage":  config.Systems[i].Usage,
				"user":   config.Systems[i].User,
				"lang":   config.Systems[i].Lang,
				"client": config.Systems[i].Client,
				"server": config.Systems[i].Server,
				"sysnr":  config.Systems[i].Sysnr,
			}).Error("missing mandatory system field(s)")
			return errors.New("checkTomlSystems(mandatory fields)")
		}

		config.Systems[i].Name = low(config.Systems[i].Name)
		config.Systems[i].Usage = low(config.Systems[i].Usage)
		config.Systems[i].Server = low(config.Systems[i].Server)
	}

	return nil
}

// establish connection to sap system
func connect(system SystemInfo, password string) (*gorfc.Connection, error) {
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
		}).Warn("Can't connect to system with user/password")
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

// exit program with error message
func exit(msg string, err error) {
	fmt.Println(msg, err)
	os.Exit(1)
}
