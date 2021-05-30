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
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/ulranh/sapnwrfc_exporter/internal"
)

type collector struct {
	// possible metric descriptions.
	Desc *prometheus.Desc

	// a parameterized function used to gather metrics.
	stats func() []metricData
}

type metricData struct {
	name       string
	help       string
	metricType string
	stats      []metricRecord
}

type metricRecord struct {
	value       float64
	labels      []string
	labelValues []string
}

// webCmd represents the web command
var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the exporter",
	Long: `With the command web you can start the sapnwrfc exporter. For example:
	sapnwrfc_exporter web
	sapnwrfc_exporter web --config ./.sapmwrfc_exporter.toml`,
	Run: func(cmd *cobra.Command, args []string) {

		config, err := getConfig()
		if err != nil {
			exit("Can't handle config file: ", err)
		}

		err = config.checkConfig()
		if err != nil {
			exit("Problems with config file: ", err)
		}

		// initialize password map
		config.passwords = make(map[string]string)

		config.Timeout, err = cmd.Flags().GetUint("timeout")
		if err != nil {
			exit("Problem with timeout flag: ", err)
		}
		config.port, err = cmd.Flags().GetString("port")
		if err != nil {
			exit("Problem with port flag: ", err)
		}

		// set data func
		// config.DataFunc = config.GetMetricData

		err = config.web()
		if err != nil {
			exit("Can't call exporter: ", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(webCmd)

	webCmd.PersistentFlags().UintP("timeout", "t", 5, "scrape timeout of the hana_sql_exporter in seconds.")
	webCmd.PersistentFlags().StringP("port", "p", "9663", "port, the hana_sql_exporter listens to.")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// webCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// webCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// create new collector
func newCollector(stats func() []metricData) *collector {
	return &collector{
		stats: stats,
	}
}

// Describe implements prometheus.Collector.
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// Collect - implements prometheus.Collector.
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	// Take a stats snapshot.  Must be concurrency safe.
	stats := c.stats()

	var valueType = map[string]prometheus.ValueType{
		"gauge":   prometheus.GaugeValue,
		"counter": prometheus.CounterValue,
	}
	for _, mi := range stats {
		for _, v := range mi.stats {
			m := prometheus.MustNewConstMetric(
				prometheus.NewDesc(mi.name, mi.help, v.labels, nil),
				valueType[mi.metricType],
				v.value,
				v.labelValues...,
			)
			ch <- m
		}
	}
}

// start collector and web server
func (config *Config) web() error {

	var err error
	// config.timeout, err = strconv.ParseUint(*flags["timeout"], 10, 0)
	// if err != nil {
	// 	exit(fmt.Sprint(" timeout flag has wrong type", err))
	// }

	// add missing system data
	config.Systems, err = config.addPasswordData()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Can't add missing system data.")
		return err
	}
	// config.timeout, err = strconv.ParseUint(*flags["timeout"], 10, 0)
	// if err != nil {
	// 	exit(fmt.Sprint(" timeout flag has wrong type", err))
	// }

	stats := func() []metricData {
		data := config.collectMetrics()
		return data
	}

	c := newCollector(stats)
	prometheus.MustRegister(c)

	// start http server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", rootHandler)

	server := &http.Server{
		Addr:         ":" + config.port,
		Handler:      mux,
		WriteTimeout: time.Duration(config.Timeout+2) * time.Second,
		ReadTimeout:  time.Duration(config.Timeout+2) * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		return errors.Wrap(err, " web - ListenAndServe")
	}

	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "prometheus sapnwrfc_exporter: please call <host>:<port>/metrics")
}

// start collecting all metrics and fetch the results
func (config *Config) collectMetrics() []metricData {

	var wg sync.WaitGroup
	mCnt := len(config.IntMetrics)
	mDataC := make(chan metricData, mCnt)

	for mPos := range config.IntMetrics {

		wg.Add(1)
		go func(mPos int) {
			defer wg.Done()
			mDataC <- metricData{
				name:       config.IntMetrics[mPos].Name,
				help:       config.IntMetrics[mPos].Help,
				metricType: config.IntMetrics[mPos].MetricType,
				stats:      config.collectSystemsMetric(mPos),
			}
		}(mPos)
	}

	go func() {
		wg.Wait()
		close(mDataC)
	}()

	var mData []metricData
	for metric := range mDataC {
		mData = append(mData, metric)
	}

	return mData
}

// start collecting metric information for all tenants
func (config *Config) collectSystemsMetric(mPos int) []metricRecord {

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(config.Timeout)*time.Second))
	defer cancel()

	sysCnt := len(config.Systems)
	mRecordsC := make(chan []metricRecord, sysCnt)

	for sPos := range config.Systems {
		go func(sPos int) {

			// all values of Metrics.TagFilter must be in Tenants.Tags, otherwise the
			// metric is not relevant for the tenant
			if !subSliceInSlice(config.IntMetrics[mPos].TagFilter, config.Systems[sPos].Tags) {
				mRecordsC <- nil
				return
			}

			servers := config.getSrvInfo(mPos, sPos)
			if servers == nil {
				mRecordsC <- nil
				return
			}

			for _, srv := range servers {
				defer srv.conn.Close()
			}
			mRecordsC <- config.collectServersMetric(mPos, sPos, servers)
		}(sPos)
	}

	var sData []metricRecord
	for i := 0; i < sysCnt; i++ {
		select {
		case mc := <-mRecordsC:

			// fmt.Println("OKE", mc)
			if mc != nil {
				sData = append(sData, mc...)
			}
		case <-ctx.Done():

			// fmt.Println("TIMEOUT!")
			return sData
		}
	}
	return sData
}

// get metric data for the system application servers
func (config *Config) collectServersMetric(mPos, sPos int, servers []serverInfo) []metricRecord {

	var wg sync.WaitGroup
	mRecordsC := make(chan []metricRecord, len(servers))

	for _, srv := range servers {

		wg.Add(1)
		go func(srv serverInfo) {
			defer wg.Done()
			mRecordsC <- config.getRfcData(mPos, sPos, srv)
		}(srv)
	}

	go func() {
		wg.Wait()
		close(mRecordsC)
	}()

	var srvData []metricRecord
	for metric := range mRecordsC {
		srvData = append(srvData, metric...)
	}

	return srvData
}

// get data from sap system
func (config *Config) getRfcData(mPos, sPos int, srv serverInfo) []metricRecord {

	// !!!!!!!!!!!!!!!
	// t := rand.Intn(5)
	// time.Sleep(time.Duration(t) * time.Second)

	// check if all configfile param keys are uppercase otherwise the function call returns an error
	for k, v := range config.IntMetrics[mPos].Params {
		upKey := up(k)
		if !(upKey == k) {
			config.IntMetrics[mPos].Params[upKey] = v
			delete(config.IntMetrics[mPos].Params, k)
		}
	}
	// call function module
	rawData, err := srv.conn.Call(up(config.IntMetrics[mPos].FunctionModule), config.IntMetrics[mPos].Params)
	if err != nil {
		log.WithFields(log.Fields{
			"system": config.Systems[sPos].Name,
			"server": srv.name,
			"error":  err,
		}).Error("Can't call function module")
		return nil
	}

	return config.IntMetrics[mPos].special.metricData(rawData, config.Systems[sPos], srv.name)
}

// retrieve table data
func (tMetric TableInfo) metricData(rawData map[string]interface{}, system SystemInfo, srvName string) []metricRecord {

	if rawData[up(tMetric.Table)] == nil {
		log.WithFields(log.Fields{
			"system": system.Name,
			"server": srvName,
			"table":  tMetric.Table,
		}).Error("metricData: no results for table")
		return nil
	}

	var md []metricRecord
	count := make(map[string]float64)

	for _, res := range rawData[up(tMetric.Table)].([]interface{}) {
		line := res.(map[string]interface{})

		if len(tMetric.RowFilter) == 0 || inFilter(line, tMetric.RowFilter) {
			for field, values := range tMetric.RowCount {
				for _, value := range values {
					namePart := low(interface2String(value))
					if "" == namePart {
						log.WithFields(log.Fields{
							"value":  namePart,
							"system": system.Name,
						}).Error("Configfile RowCount: only string and int types are allowed")
						continue
					}

					if strings.HasPrefix(low(interface2String(line[up(field)])), namePart) || "total" == namePart {
						count[low(field)+"_"+namePart]++
					}
				}
			}
		}
	}

	for field, values := range tMetric.RowCount {
		for _, value := range values {
			namePart := low(interface2String(value))

			data := metricRecord{
				labels: []string{"system", "usage", "server", "count"},
				// !!!!! low noetig?
				labelValues: []string{system.Name, system.Usage, srvName, low(field + "_" + namePart)},
				value:       count[low(field)+"_"+namePart],
			}
			md = append(md, data)
		}
	}
	return md
}

// retrieve field data
// return string fields as label
func (fMetric FieldInfo) metricData(rawData map[string]interface{}, system SystemInfo, srvName string) []metricRecord {

	var md []metricRecord

	if len(fMetric.FieldLabels) != 0 && len(fMetric.FieldValues) != 0 {
		log.Error("FieldLabels and FieldValues in one metric are not allowd")
		return nil
	}

	labels := []string{"system", "usage", "server"}
	labelValues := []string{system.Name, system.Usage, srvName}

	if len(fMetric.FieldLabels) > 0 {
		md = fMetric.getFieldLabels(rawData, labels, labelValues)
	} else {
		md = fMetric.getFieldValues(rawData, labels, labelValues)
	}
	return md

}

// field label metrics
func (fMetric FieldInfo) getFieldLabels(rawData map[string]interface{}, labels, labelValues []string) []metricRecord {

	labels = append(labels, fMetric.FieldLabels...)
	for _, label := range fMetric.FieldLabels {
		if !fieldOK(rawData, label) {
			return nil
		}
		labelValues = append(labelValues, low(rawData[up(label)].(string)))
	}

	if len(labels) != len(labelValues) {
		log.WithFields(log.Fields{
			"labels":      labels,
			"labelValues": labelValues,
		}).Error("metricData: len(labels) != len(labelValues)")
		return nil
	}
	data := metricRecord{
		labels:      labels,
		labelValues: labelValues,
		value:       1,
	}
	return []metricRecord{data}
}

// field value metrics
func (fMetric FieldInfo) getFieldValues(rawData map[string]interface{}, labels, labelValuesBase []string) []metricRecord {

	var md []metricRecord

	labels = append(labels, "field")
	for _, field := range fMetric.FieldValues {
		if !fieldOK(rawData, field) {
			return nil
		}

		f64Val, err := i2Float64(rawData[up(field)])
		if err != nil {
			log.WithFields(log.Fields{
				"field":       field,
				"field value": f64Val,
			}).Error("metricData: field value is not a correct metric value")

			continue
		}

		labelValues := append(labelValuesBase, low(field))
		data := metricRecord{
			labels:      labels,
			labelValues: labelValues,
			value:       f64Val,
		}
		md = append(md, data)
	}
	return md
}

// retrieve structure data (export structure field)
// only numbers are allowed
func (sMetric StructureInfo) metricData(rawData map[string]interface{}, system SystemInfo, srvName string) []metricRecord {

	if _, ok := rawData[up(sMetric.ExportStructure)]; !ok {
		log.WithFields(log.Fields{
			"system":          system.Name,
			"server":          srvName,
			"exportStructure": sMetric.ExportStructure,
		}).Error("metricData: exportStructure is no valid export strucure of used function module")
		return nil
	}

	var md []metricRecord
	for _, field := range sMetric.StructureFields {
		val := rawData[up(sMetric.ExportStructure)].(map[string]interface{})[up(field)]
		if val == nil {
			log.WithFields(log.Fields{
				"system":         system.Name,
				"server":         srvName,
				"structureField": field,
			}).Error("metricData: structureField is no valid export strucure field of used function module")
			return nil
		}

		f64Val, err := i2Float64(val)
		if err != nil {
			log.WithFields(log.Fields{
				"system":         system.Name,
				"server":         srvName,
				"structureField": field,
			}).Error("metricData: structureField is not a correct metric value")

			continue
		}

		labels := append([]string{"system", "usage", "server", "field"})
		labelValues := append([]string{system.Name, system.Usage, srvName, low(field)})

		data := metricRecord{
			labels:      labels,
			labelValues: labelValues,
			value:       f64Val,
		}
		md = append(md, data)
	}

	return md
}

// retrieve system servers
func (config *Config) getSrvInfo(mPos, sPos int) []serverInfo {

	c, err := connect(config.Systems[sPos], config.passwords[config.Systems[sPos].Name])
	if err != nil {
		log.WithFields(log.Fields{
			"system": config.Systems[sPos].Name,
			"error":  err,
		}).Error("No connection to sap system possible")
		return nil
	}

	params := map[string]interface{}{}
	r, err := c.Call("TH_SERVER_LIST", params)
	if err != nil {
		log.WithFields(log.Fields{
			"system": config.Systems[sPos].Name,
			"error":  err,
		}).Error("Can't call fumo th_server_list")
		return nil
	}

	// Issue 5 why is r["LIST"] == nil ?????
	if r["LIST"] == nil {
		return []serverInfo{{config.Systems[sPos].Name, c}}
	}
	srvCnt := len(r["LIST"].([]interface{}))

	// if only one server is needed for the metric
	// or if all servers are needed but only one server exists
	// -> return the standard connection. it will be closed in getRfcData.
	if !config.Metrics[mPos].AllServers || 1 == srvCnt {
		return []serverInfo{{config.Systems[sPos].Name, c}}
	}

	// if more servers exists, they get their own connection below
	// -> the standard connection has to be closed now
	c.Close()

	srvConnC := make(chan serverInfo, srvCnt)
	var wg sync.WaitGroup
	for _, v := range r["LIST"].([]interface{}) {
		wg.Add(1)

		go func(v interface{}) {
			defer wg.Done()

			appl := v.(map[string]interface{})
			if _, ok := appl["NAME"]; !ok {
				return
			}
			info := strings.Split(strings.TrimSpace(appl["NAME"].(string)), "_")

			sys := config.Systems[sPos]
			sys.Server = strings.TrimSpace(info[0])
			sys.Sysnr = strings.TrimSpace(info[2])

			srv, err := connect(sys, config.passwords[config.Systems[sPos].Name])
			if err != nil {
				log.WithFields(log.Fields{
					"server": info[0],
					"error":  err,
				}).Error("error from getServerConnections")
			} else {
				srvConnC <- serverInfo{info[0], srv}
			}
		}(v)
	}

	go func() {
		wg.Wait()
		close(srvConnC)
	}()

	var servers []serverInfo
	for server := range srvConnC {
		servers = append(servers, server)
	}

	// return connections for all servers. they will be closed in getRfcData.
	return servers
}

// add passwords and system servers to config.Systems
func (config *Config) addPasswordData() ([]SystemInfo, error) {
	var secret internal.Secret

	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		log.Fatal("Secret Values don't exist or are corrupted")
		return nil, errors.Wrap(err, " system  - Unmarshal")
	}

	var systemsOk []SystemInfo
	for _, system := range config.Systems {

		// decrypt password and add it to system config
		if _, ok := secret.Name[system.Name]; !ok {
			log.WithFields(log.Fields{
				"system": system.Name,
			}).Error("Can't find password for system")
			continue
		}
		systemsOk = append(systemsOk, system)
		pw, err := PwDecrypt(secret.Name[system.Name], secret.Name["secretkey"])
		if err != nil {
			log.WithFields(log.Fields{
				"system": system.Name,
			}).Error("Can't decrypt password for system")
			continue
		}
		// passwords = append(passwords, pw)
		config.passwords[system.Name] = pw

	}
	return systemsOk, nil
}

func i2Float64(iVal interface{}) (float64, error) {
	switch val := iVal.(type) {
	case string:
		if f64Val, err := strconv.ParseFloat(val, 64); err == nil {
			return f64Val, nil
		}
		return 42.0, errors.New("i2Float64 - string is not a number: " + val)
	case int64:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case float32:
		return float64(val), nil
	case float64:
		return float64(val), nil
	default:
	}
	return 42.0, errors.New("i2Float64 - unknown type: ")
}

// check, if toml field value, field label is valid sap field
func fieldOK(rawData map[string]interface{}, field string) bool {
	if rawData[up(field)] == nil {
		log.WithFields(log.Fields{
			"field": field,
		}).Error("metricData: field is no valid export,structure parameter of used function module")
		return false
	}
	return true
}
