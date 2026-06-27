package main

import (
	"flag"
	"log"
	"os"

	"github.com/kindbrave/claude-knowledger/internal/app"
)

func main() {
	configPath := flag.String("config", "knowledger.yaml", "path to config file")
	flag.Parse()

	var err error
	if !configFlagSet() && fileMissing(*configPath) {
		err = app.RunDefault(flag.Args())
	} else {
		err = app.Run(*configPath, flag.Args())
	}
	if err != nil {
		log.Fatal(err)
	}
}

func configFlagSet() bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			set = true
		}
	})
	return set
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
