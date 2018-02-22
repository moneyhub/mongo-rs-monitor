package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"time"
)

type replicaSet struct {
	Name          string `json:"name"`
	Members       string `json:"members"`
	MongoUser     string `json:"mongoUser"`
	MongoPwd      string `json:"mongoPwd"`
	Tls           bool   `json:"tls"`
	CheckInterval int    `json:"checkInterval"`
}

type config struct {
	SlackWebhook string       `json:"slackWebhook"`
	PagerdutyKey string       `json:"pagerdutyKey"`
	MongoUser    string       `json:"mongoUser"`
	MongoPwd     string       `json:"mongoPwd"`
	ReplicaSets  []replicaSet `json:"replicaSets"`
}

var (
	cfg        config
	configFile string
)

func readConfig(file []byte) {
	err := json.Unmarshal(file, &cfg)
	if err != nil {
		log.Fatalf("[ERROR] JSON error: %v\n", err)
	}
	if cfg.PagerdutyKey != "" {
		log.Println("[INFO] Using PagerDuty")
	}
	if cfg.SlackWebhook != "" {
		log.Println("[INFO] Using Slack")
	}
	if len(cfg.ReplicaSets) == 0 {
		log.Fatalln("[FATAL] Config invalid or does not contian valid replicaSets")
	}
}

func main() {
	if len(os.Args) == 2 {
		configFile = os.Args[1]
	} else {
		configFile = "./config/local.json"
	}

	file, e := ioutil.ReadFile(configFile)
	if e != nil {
		log.Fatalf("[ERROR] File error: %v\n", e)
	}
	readConfig(file)
	ch := make(chan replicaSet)
	for _, rs := range cfg.ReplicaSets {
		go monitor(rs, ch)
	}
	for {
		rs := <-ch
		time.Sleep(time.Duration(10) * time.Second)
		log.Printf("[INFO] Restart monitoring for %v\n", rs)
		go monitor(rs, ch)
	}
}
