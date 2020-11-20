package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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

// Describe implements prometheus.Collector.
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// Collect implements prometheus.Collector.
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
				prometheus.NewDesc(low(mi.name), mi.help, v.labels, nil),
				valueType[low(mi.metricType)],
				v.value,
				v.labelValues...,
			)
			ch <- m
		}
	}
}

func newCollector(stats func() []metricData) *collector {
	return &collector{
		stats: stats,
	}
}

// start collector and web server
func (config *Config) web(flags map[string]*string) error {

	var err error
	config.timeout, err = strconv.ParseUint(*flags["timeout"], 10, 0)
	if err != nil {
		exit(fmt.Sprint(" timeout flag has wrong type", err))
	}
	// add missing system data
	config.Systems, err = config.addPasswordData()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Can't add missing system data.")
		return err
	}
	config.timeout, err = strconv.ParseUint(*flags["timeout"], 10, 0)
	if err != nil {
		exit(fmt.Sprint(" timeout flag has wrong type", err))
	}

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
		Addr:         ":" + *flags["port"],
		Handler:      mux,
		WriteTimeout: time.Duration(config.timeout+2) * time.Second,
		ReadTimeout:  time.Duration(config.timeout+2) * time.Second,
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

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(config.timeout)*time.Second))
	defer cancel()

	// var wg sync.WaitGroup
	mCnt := len(config.metrics)
	mDataC := make(chan metricData, mCnt)

	for mPos := range config.metrics {

		// wg.Add(1)
		go func(mPos int) {
			// defer wg.Done()
			mDataC <- metricData{
				name:       low(config.metrics[mPos].Name),
				help:       config.metrics[mPos].Help,
				metricType: low(config.metrics[mPos].MetricType),
				stats:      config.collectSystemsMetric(ctx, mPos),
			}
		}(mPos)
	}

	// go func() {
	// 	wg.Wait()
	// 	close(mDataC)
	// }()

	var mData []metricData
	for i := 0; i < mCnt; i++ {
		select {
		case mc := <-mDataC:

			// ????????????? check
			// if mc != nil {
			mData = append(mData, mc)
			// }
		case <-ctx.Done():
			return mData
		}
	}
	// for metric := range mDataC {
	// 	mData = append(mData, metric)
	// }

	return mData
}

// start collecting metric information for all tenants
func (config *Config) collectSystemsMetric(ctx context.Context, mPos int) []metricRecord {
	sysCnt := len(config.Systems)
	mRecordsC := make(chan []metricRecord, sysCnt)

	for sPos := range config.Systems {
		go func(sPos int) {

			// all values of Metrics.TagFilter must be in Tenants.Tags, otherwise the
			// metric is not relevant for the tenant
			if subSliceInSlice(config.metrics[mPos].TagFilter, config.Systems[sPos].Tags) {
				servers := config.getSrvInfo(mPos, sPos)
				if servers != nil {
					for _, srv := range servers {
						defer srv.conn.Close()
					}
					mRecordsC <- config.collectServersMetric(ctx, mPos, sPos, servers)
				}
			} else {
				mRecordsC <- nil
			}
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
func (config *Config) collectServersMetric(ctx context.Context, mPos, sPos int, servers []serverInfo) []metricRecord {

	// var wg sync.WaitGroup
	srvCnt := len(servers)
	mRecordsC := make(chan []metricRecord, srvCnt)

	for _, srv := range servers {

		// wg.Add(1)
		go func(srv serverInfo) {
			// defer wg.Done()
			mRecordsC <- config.getRfcData(mPos, sPos, srv)
		}(srv)
	}

	// go func() {
	// 	wg.Wait()
	// 	close(mRecordsC)
	// }()

	var srvData []metricRecord
	for i := 0; i < srvCnt; i++ {
		select {
		case mc := <-mRecordsC:

			// ????????????? check
			// if mc != nil {
			srvData = append(srvData, mc...)
			// mData = append(mData, mc)
			// }
		case <-ctx.Done():
			return srvData
		}
	}
	// for mRecords := range mRecordsC {
	// 	if mRecords != nil {
	// 		srvData = append(srvData, mRecords...)
	// 	}
	// }

	return srvData
}

// get data from sap system
func (config *Config) getRfcData(mPos, sPos int, srv serverInfo) []metricRecord {

	// !!!!!!!!!!!!!!!
	// t := rand.Intn(5)
	// time.Sleep(time.Duration(t) * time.Second)

	// check if all configfile param keys are uppercase otherwise the function call returns an error
	for k, v := range config.metrics[mPos].Params {
		upKey := up(k)
		if !(upKey == k) {
			config.metrics[mPos].Params[upKey] = v
			delete(config.metrics[mPos].Params, k)
		}
	}
	// call function module
	rawData, err := srv.conn.Call(up(config.metrics[mPos].FunctionModule), config.metrics[mPos].Params)
	if err != nil {
		log.WithFields(log.Fields{
			"system": config.Systems[sPos].Name,
			"server": srv.name,
			"error":  err,
		}).Error("Can't call function module")
		return nil
	}

	// return table- or field metric data
	return config.metrics[mPos].metricData(rawData, config.Systems[sPos], srv.name)
}

// retrieve table data
func (tMetric tableInfo) metricData(rawData map[string]interface{}, system systemInfo, srvName string) []metricRecord {

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
				labels:      []string{"system", "usage", "server", "count"},
				labelValues: []string{low(system.Name), low(system.Usage), low(srvName), low(field + "_" + namePart)},
				value:       count[low(field)+"_"+namePart],
			}
			md = append(md, data)
		}
	}
	return md
}

// retrieve field data
// return string fields as label
func (fMetric fieldInfo) metricData(rawData map[string]interface{}, system systemInfo, srvName string) []metricRecord {

	var md []metricRecord

	if len(fMetric.FieldLabels) != 0 && len(fMetric.FieldValues) != 0 {
		log.Error("FieldLabels and FieldValues in one metric are not allowd")
		return nil
	}

	labels := []string{"system", "usage", "server"}
	labelValues := []string{low(system.Name), low(system.Usage), low(srvName)}

	if len(fMetric.FieldLabels) > 0 {
		md = fMetric.fieldLabels(rawData, labels, labelValues)
	} else {
		md = fMetric.fieldValues(rawData, labels, labelValues)
	}
	return md

}

// check, if toml field value, filed label is valid sap field
func fieldOK(rawData map[string]interface{}, field string) bool {
	if rawData[up(field)] == nil {
		log.WithFields(log.Fields{
			"field": field,
		}).Error("metricData: field is no valid export,structure parameter of used function module")
		return false
	}
	return true
}

// field label metrics
func (fMetric fieldInfo) fieldLabels(rawData map[string]interface{}, labels, labelValues []string) []metricRecord {

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
func (fMetric fieldInfo) fieldValues(rawData map[string]interface{}, labels, labelValuesBase []string) []metricRecord {

	var md []metricRecord

	labels = append([]string{"system", "usage", "server", "field"})
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
func (sMetric structureInfo) metricData(rawData map[string]interface{}, system systemInfo, srvName string) []metricRecord {

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
		labelValues := append([]string{low(system.Name), low(system.Usage), low(srvName), low(field)})

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
		return []serverInfo{serverInfo{config.Systems[sPos].Name, c}}
	}
	srvCnt := len(r["LIST"].([]interface{}))

	// if only one server is needed for the metric
	// or if all servers are needed but only one server exists
	// -> return the standard connection. it will be closed in getRfcData.
	if !config.metrics[mPos].AllServers || 1 == srvCnt {
		return []serverInfo{serverInfo{config.Systems[sPos].Name, c}}
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
func (config *Config) addPasswordData() ([]systemInfo, error) {
	var secret internal.Secret

	if err := proto.Unmarshal(config.Secret, &secret); err != nil {
		log.Fatal("Secret Values don't exist or are corrupted")
		return nil, errors.Wrap(err, " system  - Unmarshal")
	}

	var systemsOk []systemInfo
	for _, system := range config.Systems {

		// decrypt password and add it to system config
		if _, ok := secret.Name[low(system.Name)]; !ok {
			log.WithFields(log.Fields{
				"system": system.Name,
			}).Error("Can't find password for system")
			continue
		}
		systemsOk = append(systemsOk, system)
		pw, err := internal.PwDecrypt(secret.Name[low(system.Name)], secret.Name["secretkey"])
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
