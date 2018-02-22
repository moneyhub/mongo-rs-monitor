package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"gopkg.in/mgo.v2"
)

type status struct {
	currentMaster   string
	previousMaster  string
	masterAvailable bool
	unhealthyNodes  map[string]bool // true if unhealthy
	pdTriggered     map[string]bool
	slackTriggered  map[string]bool
}

func mongoCheck(mongoUsr, mongoPwd string, sess *mgo.Session) (master string, masterAvailable bool, unhealthyNodes map[string]bool) {
	masterAvailable = false
	unhealthyNodes = make(map[string]bool)
	result := make(map[string]interface{})

	admindb := sess.DB("admin")
	if mongoUsr != "" && mongoPwd != "" {
		error := admindb.Login(mongoUsr, mongoPwd)
		if error != nil {
			log.Println(error)
		}
	}
	err := admindb.Run("replSetGetStatus", &result)

	if err != nil {
		if err.Error() == "EOF" {
			log.Printf("[WARN] Connection lost. Reconnecting to rs %v\n", sess.LiveServers())
			sess.Refresh()
		} else {
			log.Fatalln("[ERROR] ", err)
		}
	} else {
		members := result["members"].([]interface{})
		for _, member := range members {
			if el, ok := member.(map[string]interface{}); ok {
				// assume node is unhealthy
				unhealthyNodes[el["name"].(string)] = true
				if el["stateStr"] == "PRIMARY" {
					masterAvailable = true
					master = el["name"].(string)
					unhealthyNodes[el["name"].(string)] = false
				} else if el["stateStr"] == "SECONDARY" {
					unhealthyNodes[el["name"].(string)] = false
				}
			}
		}
	}
	return
}

func monitor(replicaSet replicaSet, ch chan replicaSet) {

	var (
		mongoUser, mongoPwd    string
		masterUnavailableCount int
		session                *mgo.Session
		err                    error
		s                      = status{"", "", false, nil, make(map[string]bool), make(map[string]bool)}
	)

	if replicaSet.Members == "" {
		log.Fatalln("[FATAL] members value not set")
	}
	if replicaSet.Name == "" {
		replicaSet.Name = replicaSet.Members
	}

	if replicaSet.MongoUser != "" && replicaSet.MongoPwd != "" {
		mongoUser = replicaSet.MongoUser
		mongoPwd = replicaSet.MongoPwd
		log.Printf("[INFO] %s: Using Mongo authentication\n", replicaSet.Name)
	} else if cfg.MongoUser != "" && cfg.MongoPwd != "" {
		mongoUser = cfg.MongoUser
		mongoPwd = cfg.MongoPwd
		log.Printf("[INFO] %s: Using Mongo authentication\n", replicaSet.Name)
	}
	if replicaSet.CheckInterval == 0 {
		// Set default check interval
		replicaSet.CheckInterval = 10
	}
	if replicaSet.Tls == true {
		tlsConfig := &tls.Config{}
		rsSlice := strings.Split(replicaSet.Members, ",")
		dialInfo := &mgo.DialInfo{
			Addrs:    rsSlice,
			Database: "admin",
			Username: mongoUser,
			Password: mongoPwd,
		}
		dialInfo.DialServer = func(addr *mgo.ServerAddr) (net.Conn, error) {
			conn, err := tls.Dial("tcp", addr.String(), tlsConfig)
			return conn, err
		}
		session, err = mgo.DialWithInfo(dialInfo)
	} else {
		session, err = mgo.Dial(replicaSet.Members)
	}
	if err != nil {
		log.Printf("[FATAL] Connection to replicaSet %s failed: %v\n", replicaSet.Name, err)
		ch <- replicaSet
		return
	}

	defer session.Close()
	// Read from secondary
	session.SetMode(5, true)
	log.Printf("[INFO] Monitoring %s, rs members: %s\n", replicaSet.Name, replicaSet.Members)
	for {
		t := time.Now()
		s.currentMaster, s.masterAvailable, s.unhealthyNodes = mongoCheck(mongoUser, mongoPwd, session)

		if s.previousMaster != "" && s.currentMaster != "" && s.currentMaster != s.previousMaster {
			msg := fmt.Sprintf("Master has changed from %s to %s. ", s.previousMaster, s.currentMaster)
			send2slack(msg, replicaSet.Name, "danger", cfg.SlackWebhook)
		}

		// Check master availability
		if !s.masterAvailable {
			masterUnavailableCount++
			// We don't want to panic when new master is elected
			// that's why we panic only after 2 * checkInterval
			if masterUnavailableCount >= 2 {
				msg := fmt.Sprintf("Master not present")
				log.Printf("[INFO] %s: %s", replicaSet.Name, msg)
				if s.slackTriggered["master"] == false {
					slackSuccess := send2slack(msg, replicaSet.Name, "danger", cfg.SlackWebhook)
					if slackSuccess {
						s.slackTriggered["master"] = true
					}
				}
				if s.pdTriggered["master"] == false {
					pgSuccess := pg("trigger", replicaSet.Name+":masterNotAvailable", replicaSet.Name+": "+msg, cfg.PagerdutyKey)
					if pgSuccess {
						s.pdTriggered["master"] = true
					}
				}

			}
		} else {
			// Before resetting the counter notify slack and resolve PG incident
			if masterUnavailableCount >= 2 {
				slackSuccess := send2slack("Master available", replicaSet.Name, "good", cfg.SlackWebhook)
				if slackSuccess {
					s.slackTriggered["master"] = false
				}
				pgSuccess := pg("resolve", replicaSet.Name+":masterNotAvailable", replicaSet.Name+": Master available", cfg.PagerdutyKey)
				if pgSuccess {
					s.pdTriggered["master"] = false
				}
			}
			masterUnavailableCount = 0
			s.previousMaster = s.currentMaster

		}

		// Check nodes' status
		for unhealthyNodeKey, unhealthyNodeValue := range s.unhealthyNodes {
			if unhealthyNodeValue == true {
				msg := fmt.Sprintf("%s is unhealthy", unhealthyNodeKey)
				log.Println(t.String(), unhealthyNodeKey, "is unhealthy")

				if cfg.SlackWebhook != "" && !s.slackTriggered[unhealthyNodeKey] {
					slackStatus := send2slack(msg, replicaSet.Name, "danger", cfg.SlackWebhook)
					if slackStatus {
						s.slackTriggered[unhealthyNodeKey] = true
					}
				}
				if cfg.PagerdutyKey != "" && !s.pdTriggered[unhealthyNodeKey] {
					pgSuccess := pg("trigger", replicaSet.Name+":"+unhealthyNodeKey, replicaSet.Name+": "+msg, cfg.PagerdutyKey)
					if pgSuccess {
						s.pdTriggered[unhealthyNodeKey] = true
					}
				}
			}
			// Resolve the incident when mongo rs is healthy
			if unhealthyNodeValue == false {
				msg := fmt.Sprintf("%s is healthy", unhealthyNodeKey)
				if cfg.SlackWebhook != "" && s.slackTriggered[unhealthyNodeKey] == true {
					log.Println(t.String(), msg)
					success := send2slack(msg, replicaSet.Name, "good", cfg.SlackWebhook)
					if success {
						s.slackTriggered[unhealthyNodeKey] = false
					}
				}
				if cfg.PagerdutyKey != "" && s.pdTriggered[unhealthyNodeKey] == true {
					success := pg("resolve", replicaSet.Name+":"+unhealthyNodeKey, replicaSet.Name+": "+msg, cfg.PagerdutyKey)
					if success {
						s.pdTriggered[unhealthyNodeKey] = false
					}
				}
			}
		}
		time.Sleep(time.Duration(replicaSet.CheckInterval) * time.Second)
	}
}
