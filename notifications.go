package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"

	"github.com/marcw/pagerduty"
)

func send2slack(details, title, color string, webhook interface{}) (success bool) {
	text := "{\"attachments\":[{\"fallback\": \"" + title + "\", \"color\": \"" + color +
		"\", \"title\": \"" + title + "\", \"text\": \"" + details + "\" }]" +
		", \"icon_emoji\": \":warning:\",  \"as_user\": \"false\", \"username\": \"mongo-rs-monitor\"}"
	bts := []byte(text)
	body := bytes.NewBuffer(bts)
	r, err := http.Post(webhook.(string), "application/json", body)
	if err != nil {
		log.Printf("[ERROR] Send2slack: %s\n", err)
		return false
	}
	if r.StatusCode == http.StatusOK {
		return true
	}
	return false
}

func pg(eventType, incidentKey, msg string, pagerdutyKey interface{}) (success bool) {
	var event *pagerduty.Event
	pgKey := pagerdutyKey.(string)
	switch eventType {
	case "trigger":
		event = pagerduty.NewTriggerEvent(pgKey, msg)
	case "resolve":
		event = pagerduty.NewResolveEvent(pgKey, msg)
	default:
		event = pagerduty.NewTriggerEvent(pgKey, msg)
	}
	event.IncidentKey = incidentKey
	a, statusCode, err := pagerduty.Submit(event)
	fmt.Printf("%v, %v", statusCode, a)
	if err != nil {
		log.Printf("[ERROR] Pagerduty: %s\n", err)
		return false
	}
	if statusCode == http.StatusOK {
		return true
	}
	return false

}
