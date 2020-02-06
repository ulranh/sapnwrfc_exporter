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
)

// system info
type systemInfo struct {
	Name     string
	Usage    string
	Tags     []string
	User     string
	Lang     string
	Client   string
	Server   string // umbenennen !!!!!
	Sysnr    string
	password string
	servers  []serverInfo
}

type serverInfo struct {
	name  string
	sysnr string
}

// metric info
type metricInfo struct {
	// Active       bool
	Name       string
	Help       string
	MetricType string
	TagFilter  []string
	FuMo       string
	Params     map[string]interface{}
	Table      string
	AllServers bool // ????
	RowCount   map[string][]interface{}
	RowFilter  map[string][]interface{}
}

// Config struct with config file infos
type Config struct {
	Secret       []byte
	Systems      []*systemInfo
	TableMetrics []*metricInfo
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
	errConfsystemMissing = errors.New("\nthe system info is missing.")
	errConfMetricMissing = errors.New("\nthe metric info is missing.")
	// !!!!!!!!!!!!!!!!!!!!!!!!! anpassen
	errConfsystem     = errors.New("\nat least one of the required system fields Name,ConnStr or User is empty.")
	errConfMetric     = errors.New("\nat least one of the required metric fields Name,Help,MetricType or SQL is empty.")
	errConfMetricType = errors.New("\nat least one of the existing MetricType fields does not contain the allowed content counter or gauge.")
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
		params: []string{"config", "port"},
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
	var config Config
	if _, err := toml.DecodeFile(*flags["config"], &config); err != nil {
		exit(fmt.Sprint("Problem with configfile decoding: ", err))
	}

	// parse config file
	if err = config.parseConfigInfo(command); err != nil {
		exit(fmt.Sprint("Problem with configfile content: ", err))
	}
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
		return errConfsystemMissing
	}
	if 0 == len(config.TableMetrics) {
		return errConfMetricMissing
	}
	for _, system := range config.Systems {
		if 0 == len(system.Name) || 0 == len(system.Usage) || 0 == len(system.User) || 0 == len(system.Lang) || 0 == len(system.Client) || 0 == len(system.Server) || 0 == len(system.Sysnr) {
			return errConfsystem
		}
	}
	for _, metric := range config.TableMetrics {
		if 0 == len(metric.Name) || 0 == len(metric.Help) || 0 == len(metric.MetricType) || 0 == len(metric.FuMo) || 0 == len(metric.Table) {
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
