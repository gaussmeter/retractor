// https://www.eclipse.org/paho/clients/golang/

/*
Todo
make configurable....
-- log level
*/

package main

import (
	"os"
	"strconv"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	ag "github.com/gaussmeter/mqttagent"
	log "github.com/sirupsen/logrus"
	randstr "github.com/thanhpk/randstr"
)

var debug bool = true

var geoFence string = "unset"
var chargeDoor bool = false
var chargerDirection string = "down"

var lastGeoFence string = "unset"
var lastChargeDoor bool = false
var lastChargerDirection string = "unset"


var host string = "ws://192.168.1.51:9001"
var car string = "1"
var home string = "Home"
var topicPrefix string = "teslamate/cars/"
var lumenHost string = "http://192.168.1.127:9000/lumen"
var user string = ""
var pass string = ""
var loopSleep time.Duration = 250


var geoFenceMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	geoFence = string(msg.Payload())
}
var chargeDoorMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	chargeDoor, _ = strconv.ParseBool(string(msg.Payload()))
}

func getSetting(setting string, defaultValue string) (value string) {
        if os.Getenv(setting) != "" {
                log.WithFields(log.Fields{"configFrom": "env", setting: os.Getenv(setting)}).Info()
                return os.Getenv(setting)
        }
        log.WithFields(log.Fields{"configFrom": "default", setting: defaultValue}).Info()
        return defaultValue
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})
	// get config from environment
	log.SetReportCaller(false)
	host = getSetting("MQTT_HOST", host)
	user = getSetting("MQTT_USER", user)
	pass = getSetting("MQTT_PASS", pass)
	lumenHost = getSetting("LUMEN_HOST", lumenHost)
	car = getSetting("CAR_NUMBER", car)
	home = getSetting("GEOFENCE_HOME", home)
	// default to home/open -> failsafe to make sure the charger drops down
	geoFence = home
	chargeDoor = true
}

func main() {

	agent := ag.NewAgent(host, "charger-"+randstr.String(4), user, pass)
	err := agent.Connect()
	if err != nil {
		log.WithField("error", err).Error("Can't connect to mqtt server")
		os.Exit(1)
	}
	agent.Subscribe(topicPrefix+car+"/geofence", geoFenceMq)
	agent.Subscribe(topicPrefix+car+"/charge_port_door_open", chargeDoorMq)
	log.WithFields(log.Fields{"chargeDoor": chargeDoor, "geoFence":geoFence}).Info("Initial Values")
	lastChargeDoor = chargeDoor
	lastGeoFence = geoFence

	for !agent.IsTerminated() {
		switch true {
		case geoFence == home && chargeDoor:
		        chargerDirection = "down"
			break
		case geoFence != home || !chargeDoor:
		        chargerDirection = "up"
			break
		}
		if geoFence != lastGeoFence || chargeDoor != lastChargeDoor {
			lastChargeDoor = chargeDoor
			lastGeoFence = geoFence
	                log.WithFields(log.Fields{"chargeDoor": chargeDoor, "geoFence":geoFence}).Info("Changed Values")
		}
		if chargerDirection != lastChargerDirection {
			lastChargerDirection = chargerDirection
			log.WithFields(log.Fields{"chargerDirection":chargerDirection}).Info("Direction change")
		}
		time.Sleep(loopSleep * time.Millisecond)
	}

}
