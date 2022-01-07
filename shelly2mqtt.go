package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"golang.org/x/net/trace"
)

var (
	listenAddress = flag.String("listen",
		":8773",
		"listen address for HTTP API (e.g. for Shelly buttons)")

	mqttBroker = flag.String("mqtt_broker",
		"tcp://dr.lan:1883",
		"MQTT broker address for github.com/eclipse/paho.mqtt.golang")

	mqttPrefix = flag.String("mqtt_topic",
		"github.com/stapelberg/shelly2mqtt/",
		"MQTT topic prefix")
)

func shelly2mqtt() error {
	opts := mqtt.NewClientOptions().AddBroker(*mqttBroker)
	clientID := "https://github.com/stapelberg/shelly2mqtt"
	if hostname, err := os.Hostname(); err == nil {
		clientID += "@" + hostname
	}
	opts.SetClientID(clientID)
	opts.SetConnectRetry(true)
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("MQTT connection failed: %v", token.Error())
	}

	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/requests/", trace.Traces)
	mux.HandleFunc("/door/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("http: %s", r.URL.Path)
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/door/"), "/")
		if len(parts) != 2 {
			log.Printf("parts = %q", parts)
			return
		}
		room := parts[0]
		command := parts[1]
		// command == "off" means door opened
		// command == "on" means door closed
		log.Printf("room %q, command %q", room, command)

		b, err := json.Marshal(struct {
			Onoff bool `json:"onoff"`
		}{
			// TODO: reverse semantics here and in regelwerk once zigbee sensors
			// are no longer in use
			Onoff: command == "off",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Print(err)
			return
		}

		mqttClient.Publish(
			*mqttPrefix+"door/"+room,
			0,    /* qos */
			true, /* retained */
			string(b))
		log.Printf("published to MQTT")
	})

	log.Printf("http.ListenAndServe(%q)", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, mux); err != nil {
		return err
	}
	return nil
}

func main() {
	flag.Parse()
	if err := shelly2mqtt(); err != nil {
		log.Fatal(err)
	}
}
