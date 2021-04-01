// https://www.eclipse.org/paho/clients/golang/

/*
Todo
make configurable....
-- log level
*/

package main

import (
	"os"
	"runtime"
	"strconv"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	ag "github.com/gaussmeter/mqttagent"
	log "github.com/sirupsen/logrus"
	rpio "github.com/stianeikeland/go-rpio/v4"
	randstr "github.com/thanhpk/randstr"
)

var debug bool = true

var geoFence string = ""
var chargeDoor string = ""
var chargerDirection string = ""

var lastGeoFence string = ""
var lastChargeDoor string = ""
var lastChargerDirection string = ""

var lgf string = ""

var carState string = ""

var host string = "ws://192.168.1.51:9001"
var car string = "1"
var home string = "Home"
var topicPrefix string = "teslamate/cars/"
var user string = ""
var pass string = ""
var loopSleep time.Duration = 250

var upPin = rpio.Pin(27)
var downPin = rpio.Pin(17)
var enablePin = rpio.Pin(22)

var led1Pin = rpio.Pin(23)
var led2Pin = rpio.Pin(24)

var eStopPin = rpio.Pin(26)
var eStop bool = false

var lastBlink int64 = time.Now().Unix()

var geoFenceMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	geoFence = string(msg.Payload())
	if geoFence == "" {
		geoFence = "empty"
	}
	if geoFence != lgf {
		log.WithFields(log.Fields{"geoFence": geoFence}).Info("MQTT")
		lgf = geoFence
	}
}
var chargeDoorMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	chargeDoorOpen, _ := strconv.ParseBool(string(msg.Payload()))
	if chargeDoorOpen {
		chargeDoor = "open"
	} else {
		chargeDoor = "closed"
	}
	log.WithFields(log.Fields{"chargeDoor": chargeDoor, "charge_port_door_open": chargeDoorOpen}).Info("MQTT")
}
var carStateMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	carState = string(msg.Payload())
	log.WithFields(log.Fields{"carState": carState}).Info("MQTT")
}

func getSetting(setting string, defaultValue string) (value string) {
	if os.Getenv(setting) != "" {
		log.WithFields(log.Fields{"configFrom": "env", setting: os.Getenv(setting)}).Info()
		return os.Getenv(setting)
	}
	log.WithFields(log.Fields{"configFrom": "default", setting: defaultValue}).Info("Settings")
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
	car = getSetting("CAR_NUMBER", car)
	home = getSetting("GEOFENCE_HOME", home)
	if runtime.GOARCH == "arm" {
		_ = rpio.Open()
		upPin.Output()
		downPin.Output()
		enablePin.Output()
		led1Pin.Output()
		led2Pin.Output()
		eStopPin.Input()
		eStopPin.PullUp()
	}
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
	agent.Subscribe(topicPrefix+car+"/state", carStateMq)

	for !agent.IsTerminated() {
		switch true {
		case ((geoFence == home && chargeDoor == "open") && (geoFence != "" && chargeDoor != "")) || eStop:
			chargerDirection = "down"
			if runtime.GOARCH == "arm" {
				enablePin.High()
				upPin.Low()
				downPin.High()
				led2Pin.High()
			}
			break
		case (geoFence != home || chargeDoor == "closed") && (geoFence != "" && chargeDoor != "") && !eStop:
			chargerDirection = "up"
			if runtime.GOARCH == "arm" {
				enablePin.High()
				downPin.Low()
				upPin.High()
				led2Pin.Low()
			}
			break
		}
		if geoFence != lastGeoFence || chargeDoor != lastChargeDoor {
			lastChargeDoor = chargeDoor
			lastGeoFence = geoFence
			log.WithFields(log.Fields{"chargeDoor": chargeDoor, "geoFence": geoFence}).Info("State")
		}
		if chargerDirection != lastChargerDirection {
			lastChargerDirection = chargerDirection
			log.WithFields(log.Fields{"chargerDirection": chargerDirection}).Info("Charger")
		}
		if eStop {
			// blink led1Pin
			if time.Now().Unix()-lastBlink > 1 {
				lastBlink = time.Now().Unix()
				led1Pin.Toggle()
			}
		} else {
			if carState == "online" {
				if runtime.GOARCH == "arm" {
					led1Pin.High()
				}
			} else {
				if runtime.GOARCH == "arm" {
					led1Pin.Low()
				}
			}
		}
		if runtime.GOARCH == "arm" {
			if eStopPin.Read() == rpio.Low && !eStop {
				eStop = true
				log.Info("E-Stop!")
			}
		}
		time.Sleep(loopSleep * time.Millisecond)
	}

}
