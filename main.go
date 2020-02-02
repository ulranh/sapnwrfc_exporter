package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/ulranh/sapnwrfc_exporter/cmd"
)

func main() {
	Formatter := new(log.TextFormatter)
	Formatter.TimestampFormat = "02-01-2006 15:04:05"
	// Formatter.FullTimestamp = true
	// log.SetFormatter(Formatter)
	log.SetFormatter(&log.TextFormatter{
		DisableColors:   true,
		FullTimestamp:   true,
		TimestampFormat: "02-01-2006 15:04:05",
	})
	// log.Info("Some info. Earth is not flat.")
	// log.Warning("This is a warning")
	// log.Error("Not fatal. An error. Won't stop execution")
	// log.Fatal("MAYDAY MAYDAY MAYDAY. Execution will be stopped here")
	// log.Panic("Do not panic")

	cmd.Root()
}
