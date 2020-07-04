package cmd

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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
	err = config.addSystemData()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Can't add missing system data.")
		return err
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
	server := &http.Server{
		Addr:         ":" + *flags["port"],
		Handler:      mux,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		return errors.Wrap(err, " web - ListenAndServe")
	}

	return nil
}

// start collecting all metrics and fetch the results
func (config *Config) collectMetrics() []metricData {

	mDataC := make(chan metricData, len(config.metrics))
	// metricsC := make(chan metricData)
	var wg sync.WaitGroup

	// wg.Add(len(config.Metrics))
	for mPos := range config.metrics {
		// log.Println("1111111111111111111: ", config.metrics[mPos])

		wg.Add(1)
		go func(mPos int) {
			defer wg.Done()
			// !!!!!! zusammenfassen
			mDataC <- metricData{
				name:       config.metrics[mPos].Name,
				help:       config.metrics[mPos].Help,
				metricType: config.metrics[mPos].MetricType,
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
	mRecordsC := make(chan []metricRecord, len(config.Systems))
	// metricC := make(chan []statData)
	var wg sync.WaitGroup

	for sPos := range config.Systems {
		wg.Add(1)
		go func(sPos int) {
			defer wg.Done()
			mRecordsC <- config.collectServersMetric(mPos, sPos)
		}(sPos)
	}

	go func() {
		wg.Wait()
		close(mRecordsC)
	}()

	var sData []metricRecord
	for mRecords := range mRecordsC {
		sData = append(sData, mRecords...)
	}
	return sData
}

// get metric data for all systems application servers
func (config *Config) collectServersMetric(mPos, sPos int) []metricRecord {

	servers := config.Systems[sPos].servers
	if !config.metrics[mPos].AllServers {
		// only one server is needed
		servers = config.Systems[sPos].servers[:1]
	}
	log.Println("333333333333333: ", config.metrics[mPos].Name, servers)

	srvCnt := len(servers)
	mRecordsC := make(chan []metricRecord, srvCnt)

	// !!!!!!!!!!!!!!! fuer server auch den index nehmen
	for srvPos := range servers {

		go func(srvPos int) {
			mRecordsC <- config.getRfcData(mPos, sPos, srvPos)
		}(srvPos)

	}

	i := 0
	var srvData []metricRecord
	timeAfter := time.After(time.Duration(config.timeout) * time.Second)

stopReading:
	for {
		select {
		case mc := <-mRecordsC:
			if mc != nil {
				srvData = append(srvData, mc...)
			}
			i += 1
			if srvCnt == i {
				break stopReading
			}
		case <-timeAfter:
			break stopReading
		}
	}
	// log.Println("222222222222222: ", sData)
	return srvData
}

func (config *Config) getRfcData(mPos, sPos, srvPos int) []metricRecord {

	// connect to system/server
	c, err := connect(config.Systems[sPos], config.Systems[sPos].servers[srvPos])
	if err != nil {
		return nil
	}
	defer c.Close()

	// all values of Metrics.TagFilter must be in Tenants.Tags, otherwise the
	// metric is not relevant for the tenant
	if !subSliceInSlice(config.metrics[mPos].TagFilter, config.Systems[sPos].Tags) {
		return nil
	}

	// call metrics function module
	raw, err := c.Call(config.metrics[mPos].FunctionModule, config.metrics[mPos].Params)
	if err != nil {
		log.WithFields(log.Fields{
			"system": config.Systems[sPos].Name,
			"server": config.Systems[sPos].servers[srvPos].name,
			"error":  err,
		}).Error("Can't call function module")
		return nil
	}
	// log.Println(raw)
	return config.metrics[mPos].metricData(raw, config.Systems[sPos], config.Systems[sPos].servers[srvPos].name)
}

func (tMetric tableInfo) metricData(rawData map[string]interface{}, system *systemInfo, srvName string) []metricRecord {
	var md []metricRecord
	count := make(map[string]float64)

	for _, res := range rawData[tMetric.Table].([]interface{}) {
		line := res.(map[string]interface{})

		if len(tMetric.RowFilter) == 0 || inFilter(line, tMetric.RowFilter) {
			for field, values := range tMetric.RowCount {
				for _, value := range values {
					namePart := low(interface2String(value))
					if "" == namePart {
						// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
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

func (fMetric fieldInfo) metricData(rawData map[string]interface{}, system *systemInfo, srvName string) []metricRecord {

	var fieldLabelValues []string
	for _, label := range fMetric.FieldLabels {
		fieldLabelValues = append(fieldLabelValues, rawData[up(label)].(string))
	}

	var md []metricRecord
	labels := append([]string{"system", "usage", "server"}, fMetric.FieldLabels...)
	labelValues := append([]string{low(system.Name), low(system.Usage), low(srvName)}, fieldLabelValues...)

	if len(labels) != len(labelValues) {
		log.WithFields(log.Fields{
			"system": system.Name,
			"server": srvName,
		}).Error("getRfcData: len(labels) != len(labelValues)")
		return nil

	}

	data := metricRecord{
		labels:      labels,
		labelValues: labelValues,
		value:       1,
	}
	md = append(md, data)
	return md
}
