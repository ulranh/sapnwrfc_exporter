package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/ulranh/sapnwrfc_exporter/internal"
)

func Test_cmdParams(t *testing.T) {

	cf, err := ioutil.TempFile(os.TempDir(), "config-")
	if err != nil {
		log.Fatal("Cannot create temporary file", err)
	}
	defer os.Remove(cf.Name())
	fmt.Println(cf.Name())

	var cmdInfo = []struct {
		args  []string
		cmd   string
		flags map[string]string
		err   error
	}{
		// no cmd
		{[]string{"sapnwrfc_exporter"}, "", nil, errCmdNotGiven},
		// wrong cmd
		{[]string{"sapnwrfc_exporter", "a"}, "", nil, errCmdNotAvailable},
		// wrong cmd before correct cmd
		{[]string{"sapnwrfc_exporter", "a", "pw"}, "", nil, errCmdNotAvailable},
		// wrong cmd after correct cmd
		{[]string{"sapnwrfc_exporter", "pw", "a"}, "", nil, errCmdFlagMissing},
		// configfile does not exist
		{[]string{"sapnwrfc_exporter", "pw", "-system", "p01", "-config", "nothere.toml"}, "", nil, errCmdFileMissing},
		// configfile does exist
		{[]string{"sapnwrfc_exporter", "pw", "-system", "p01", "-config", cf.Name()}, "pw", map[string]string{
			"system": "p01",
			"config": cf.Name(),
		}, nil},
		{[]string{"sapnwrfc_exporter", "web", "-port", "3232", "-config", cf.Name()}, "web", map[string]string{
			"port":   "3232",
			"config": cf.Name(),
		}, nil},
	}

	for _, line := range cmdInfo {
		cmd, flags, err := getCmdInfo(line.args)
		internal.Equals(t, cmd, line.cmd)
		for k := range line.flags {
			internal.Equals(t, *flags[k], line.flags[k])
		}
		internal.Equals(t, err, line.err)
	}

	// Close tmpfile
	if err := cf.Close(); err != nil {
		log.Fatal("Cannot close temporary file", err)
	}
}
