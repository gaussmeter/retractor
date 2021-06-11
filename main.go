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

var preDrop bool = false
var preDropStart int64 = time.Now().Unix()

var geoFence string = ""
var chargeDoor string = ""
var chargerDirection string = ""
var heading bool = false
var shiftState string = ""

var lastGeoFence string = ""
var lastChargeDoor string = ""
var lastChargerDirection string = ""
var lastShiftState string = ""

var lgf string = ""
var lh bool = false

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

var headingMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	currentHeading, _ := strconv.Atoi(string(msg.Payload()))
	if currentHeading > 340 && currentHeading < 360 && geoFence == home {
		heading = true
	} else {
		heading = false
	}
	if heading != lh {
		log.WithFields(log.Fields{"currentHeading": currentHeading, "heading": heading}).Info("MQTT")
	}
}

var shiftStateMq MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	shiftState = string(msg.Payload())
	log.WithFields(log.Fields{"shiftState": shiftState}).Info("MQTT")
}

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
	log.WithFields(log.Fields{"GOARCH": runtime.GOARCH}).Info("init")
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
	agent.Subscribe(topicPrefix+car+"/heading", headingMq)
	agent.Subscribe(topicPrefix+car+"/shift_state", shiftStateMq)

	for !agent.IsTerminated() {
		switch true {
		case eStop:
			chargerDirection = "down"
			break
		case preDrop:
			chargerDirection = "down"
			break
		case (geoFence != home && chargeDoor == "open"):
			// ocassionally Teslamte sends an empty geofence
			// do nothing as long as the doore is open.
			break
		case (geoFence == home && chargeDoor == "open") && (geoFence != "" && chargeDoor != ""):
			chargerDirection = "down"
			break
		case (geoFence != home || chargeDoor == "closed") && (geoFence != "" && chargeDoor != "") && !eStop:
			chargerDirection = "up"
			break
		}
		retractor(chargerDirection)
		if preDropStart+300 < time.Now().Unix() && preDrop == true {
			preDrop = false
			log.WithFields(log.Fields{"preDrop": preDrop}).Info("Charger")
		}
		if geoFence == home && heading == true && shiftState == "R" && preDrop == false {
			preDrop = true
			preDropStart = time.Now().Unix()
			log.WithFields(log.Fields{"preDrop": preDrop}).Info("Charger")
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

func retractor(direction string) {
	if runtime.GOARCH == "arm" {
		switch direction {
		case "down":
			enablePin.High()
			upPin.Low()
			downPin.High()
			led2Pin.High()
			break
		case "up":
			enablePin.High()
			downPin.Low()
			upPin.High()
			led2Pin.Low()
			break
		}
	}
}
