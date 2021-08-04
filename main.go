package main

import (
	"flag"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

var configFlag = flag.String("config", "concert.yml", "Specify path of the config file")

func main() {
	flag.Parse()
	configFile, err := os.Open(*configFlag)
	if err != nil {
		log.Fatalln(err)
	}
	decoder := yaml.NewDecoder(configFile)
	var config ConcertConfig
	err = decoder.Decode(&config)
	concert, err := NewConcert(&config)
	if err != nil {
		log.Fatalln(err)
	}
	err = concert.Run()
	if err != nil {
		log.Fatalln(err)
	}
}
