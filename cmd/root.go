package cmd

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sap/gorfc/gorfc"
)

// server information
type serverInfo struct {
	name string
	conn *gorfc.Connection
}

// system information
type systemInfo struct {
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
type metricInfo struct {
	Name           string
	Help           string
	MetricType     string
	TagFilter      []string
	AllServers     bool
	FunctionModule string
	Params         map[string]interface{}
}

// specific table metric info
type tableInfo struct {
	Table     string
	RowCount  map[string][]interface{}
	RowFilter map[string][]interface{}
}

// specific field metric info
type fieldInfo struct {
	FieldLabels []string
}

type structureInfo struct {
	ExportStructure string
	StructureFields []string
}

// interface for different handling of table- and field metrics
type dataReceiver interface {
	metricData(rawData map[string]interface{}, system systemInfo, srvName string) []metricRecord
}

type tableMetric struct {
	metricInfo
	tableInfo
}

type fieldMetric struct {
	metricInfo
	fieldInfo
}

type structureMetric struct {
	metricInfo
	structureInfo
}

type entireMetric struct {
	metricInfo
	dataReceiver
}

// config information for the whole process
type Config struct {
	Secret           []byte
	Systems          []systemInfo      // system info from toml file
	TableMetrics     []tableMetric     // table metric info from toml file
	FieldMetrics     []fieldMetric     // field metric info from toml file
	StructureMetrics []structureMetric // structure metric info from toml file
	metrics          []entireMetric    // table- and field-info condensed
	passwords        map[string]string
	timeout          uint64
}

// help text for command usage
const doc = `usage: sapnwrfc_exporter <command> <param> [param ...]

These are the commands:
  pw              Set password for system connection(s)
  web             Start Prometheus web client

Run 'sapnwrfc_exporter <command> -help' to see the corresponding parameters.
`

var (
	errCmdNotGiven       = errors.New("\nCmd Problem: command ist not given.")
	errCmdNotAvailable   = errors.New("\nCmd Problem: command ist not available.")
	errCmdFlagMissing    = errors.New("\nCmd Problem: a required cmd flag is missing.")
	errCmdFileMissing    = errors.New("\nCmd Problem: no configfile found.")
	errConfSecretMissing = errors.New("\nthe secret info is missing. Please add the passwords with \"sapnwrfc_exporter pw -system <system>\"")
	errConfSystemMissing = errors.New("\nthe system info is missing.")
	errConfMetricMissing = errors.New("\nthe metric info is missing.")
	errConfSystem        = errors.New("\nat least one of the required system fields Name,Usage,Lang,Server,Sysnr,Client or User is empty.")
	errConfMetric        = errors.New("\nat least one of the required metric fields Name,Help,MetricType or FunctionModule is empty.")
	errConfMetricType    = errors.New("\nat least one of the existing MetricType fields does not contain the allowed content counter or gauge.")
)

// map of allowed parameters
var paramMap = map[string]*struct {
	value string
	usage string
}{
	"system": {
		value: "",
		usage: "Name(s) of system(s) - lower case words separated by comma (required)\ne.g. -system P01,P02",
	},
	"port": {
		value: "9663",
		usage: "Client port",
	},
	"config": {
		value: "",
		usage: "Path + name of toml config file",
	},
	"timeout": {
		value: "10",
		usage: "timeout of the hana connector in seconds",
	},
}

// map of allowed commands
var commandMap = map[string]struct {
	defaultParams map[string]string
	params        []string
	// help          string
}{
	"pw": {
		params: []string{"config", "system"},
		// help:   pwHelp,
	},
	"web": {
		params: []string{"config", "port", "timeout"},
		// help:   wHelp,
	},
}

// Root checks given command, flags and config file. If ok jump to corresponding execution
func Root() {

	// check command and flags
	command, flags, err := getCmdInfo(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// decode config file
	passwords := make(map[string]string)
	var config Config
	if _, err := toml.DecodeFile(*flags["config"], &config); err != nil {
		exit(fmt.Sprint("Problem with configfile decoding: ", err))
	}

	// consollidate the different metric types
	for _, m := range config.TableMetrics {
		config.metrics = append(config.metrics, entireMetric{m.metricInfo, m.tableInfo})
	}
	for _, m := range config.FieldMetrics {
		config.metrics = append(config.metrics, entireMetric{m.metricInfo, m.fieldInfo})
	}
	for _, m := range config.StructureMetrics {
		config.metrics = append(config.metrics, entireMetric{m.metricInfo, m.structureInfo})
	}

	// parse config file
	if err = config.parseConfigInfo(command); err != nil {
		exit(fmt.Sprint("Problem with configfile content: ", err))
	}

	// add password map to config
	config.passwords = passwords

	// run cmd
	var cmdFunc = map[string]func(map[string]*string) error{
		"pw":  config.pw,
		"web": config.web,
	}

	err = cmdFunc[command](flags)
	if err != nil {
		exit(fmt.Sprint("Error from command with ", err))
	}
}

// check command and args
func getCmdInfo(args []string) (string, map[string]*string, error) {

	// no command given
	if 1 == len(args) {
		fmt.Print(doc)
		return "", nil, errCmdNotGiven
	}

	// command name is incorrect
	if _, ok := commandMap[args[1]]; !ok {
		fmt.Print(doc)
		return "", nil, errCmdNotAvailable
	}

	// initial path for config file
	home, err := homedir.Dir()
	if err != nil {
		return "", nil, errors.Wrap(err, "can't detect homdir for config file")
	}
	paramMap["config"].value = path.Join(home, args[0]+".toml")

	// create and fill flag set
	flags := make(map[string]*string)
	c := flag.NewFlagSet(args[1], flag.ExitOnError)
	for _, v := range commandMap[args[1]].params {
		flags[v] = c.String(v, paramMap[v].value, paramMap[v].usage)
	}
	c.SetOutput(os.Stderr)

	// parse flags
	err = c.Parse(args[2:])
	if err != nil {
		return "", nil, errors.Wrap(err, "getCmdInfo - problem with c.Parse()")
	}
	if c.Parsed() {

		// break if default flag system for command pw ist missing
		if name, ok := flags["system"]; ok {
			if *name == "" {
				c.PrintDefaults()
				return "", nil, errCmdFlagMissing
			}
		}

		// break if there exists no configfile at the given default path
		if name, ok := flags["config"]; ok {
			if _, err := os.Stat(*name); os.IsNotExist(err) {
				c.PrintDefaults()
				return "", nil, errCmdFileMissing
			}
		}
	}

	return args[1], flags, nil
}

// check configfile
func (config *Config) parseConfigInfo(cmd string) error {
	if cmd != "web" {
		return nil
	}

	if 0 == len(config.Secret) {
		return errConfSecretMissing
	}
	if 0 == len(config.Systems) {
		return errConfSystemMissing
	}
	if 0 == len(config.metrics) {
		return errConfMetricMissing
	}
	for _, system := range config.Systems {
		// if 0 == len(system.Name) || 0 == len(system.Usage) || 0 == len(system.User) || 0 == len(system.Lang) || 0 == len(system.Client) || 0 == len(system.Server) || 0 == len(system.Sysnr) {
		// !!!!!!!!!!!!! adapt
		if 0 == len(system.Name) || 0 == len(system.Usage) || 0 == len(system.User) || 0 == len(system.Lang) || 0 == len(system.Client) {
			return errConfSystem
		}
	}
	for _, metric := range config.metrics {
		if 0 == len(metric.Name) || 0 == len(metric.Help) || 0 == len(metric.MetricType) || 0 == len(metric.FunctionModule) {
			return errConfMetric
		}
		if !strings.EqualFold(metric.MetricType, "counter") && !strings.EqualFold(metric.MetricType, "gauge") {
			return errConfMetricType
		}
	}
	return nil
}

func exit(msg string) {
	fmt.Println(msg)
	os.Exit(1)
}
