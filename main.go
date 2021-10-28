package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	vallox "github.com/pvainio/vallox-rs485"

	"github.com/kelseyhightower/envconfig"

	mqttClient "github.com/eclipse/paho.mqtt.golang"
)

type cacheEntry struct {
	time  time.Time
	value vallox.Event
}

const (
	topicFanSpeed            = "vallox/fan/speed"
	topicTempIncomingIside   = "vallox/temp/incoming/inside"
	topicTempIncomingOutside = "vallox/temp/incoming/outside"
	topicTempOutgoingInside  = "vallox/temp/outgoing/inside"
	topicTempOutgoingOutside = "vallox/temp/outgoing/outside"
)

var topicMap = map[byte]string{
	vallox.FanSpeed:            topicFanSpeed,
	vallox.TempIncomingInside:  topicTempIncomingIside,
	vallox.TempIncomingOutside: topicTempIncomingOutside,
	vallox.TempOutgoingInside:  topicTempOutgoingInside,
	vallox.TempOutgoingOutside: topicTempOutgoingOutside,
}

var (
	logDebug *log.Logger
	logInfo  *log.Logger
	logError *log.Logger
)

type Config struct {
	SerialDevice string `envconfig:"serial_device" required:"true"`
	MqttUrl      string `envconfig:"mqtt_url" required:"true"`
	MqttUser     string `envconfig:"mqtt_user"`
	MqttPwd      string `envconfig:"mqtt_password"`
	MqttClientId string `envconfig:"mqtt_client_id" default:"vallox"`
	Debug        bool   `envconfig:"debug" default:"false"`
}

var config Config

func init() {

	err := envconfig.Process("vallox", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	initLogging()
}

func main() {

	mqtt := connectMqtt()

	valloxDevice := connectVallox()

	cache := make(map[byte]cacheEntry)

	announceMe(mqtt, cache)

	initHAStatusHandler(mqtt, cache)

	for {
		handleEvent(valloxDevice, <-valloxDevice.Events(), cache, mqtt)
	}
}

func initHAStatusHandler(mqtt mqttClient.Client, cache map[byte]cacheEntry) {
	mqtt.Subscribe("homeassistant/status", 0, createHAStatusHandler(mqtt, cache))
}

func createHAStatusHandler(mqtt mqttClient.Client, cache map[byte]cacheEntry) func(mqttClient.Client, mqttClient.Message) {
	return func(mqtt mqttClient.Client, msg mqttClient.Message) {
		body := string(msg.Payload())
		logInfo.Printf("received HA status %s", body)
		if body == "online" {
			// ha became online, send discovery
			go announceMe(mqtt, cache)
		} else if body != "offline" {
			logInfo.Printf("unknown HA status message %s", body)
		}
	}
}

func connectVallox() *vallox.Vallox {
	cfg := vallox.Config{Device: config.SerialDevice}

	logInfo.Printf("connecting to vallox serial port %s", cfg.Device)

	valloxDevice, err := vallox.Open(cfg)

	if err != nil {
		logError.Fatalf("error opening Vallox device %s: %v", config.SerialDevice, err)
	}

	return valloxDevice
}

func connectMqtt() mqttClient.Client {

	opts := mqttClient.NewClientOptions().
		AddBroker(config.MqttUrl).
		SetClientID(config.MqttClientId).
		SetOrderMatters(false).
		SetKeepAlive(150 * time.Second).
		SetAutoReconnect(true).
		SetConnectionLostHandler(connectionLostHandler).
		SetOnConnectHandler(connectHandler).
		SetReconnectingHandler(reconnectHandler)

	if len(config.MqttUser) > 0 {
		opts = opts.SetUsername(config.MqttUser)
	}

	if len(config.MqttPwd) > 0 {
		opts = opts.SetPassword(config.MqttPwd)
	}

	logInfo.Printf("connecting to mqtt %s client id %s user %s", opts.Servers, opts.ClientID, opts.Username)

	c := mqttClient.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	return c
}

func handleEvent(vallox *vallox.Vallox, e vallox.Event, cache map[byte]cacheEntry, mqtt mqttClient.Client) {
	if !vallox.ForMe(e) {
		// Ignore values not addressed for me
		return
	}

	now := time.Now()
	validTime := now.Add(time.Duration(-15) * time.Minute)

	if val, ok := cache[e.Register]; !ok {
		// First time we receive this value, send Home Assistant discovery
		announceRawData(mqtt, e.Register)
	} else if val.value.RawValue == e.RawValue && val.time.After(validTime) {
		// we already have that value and have recently published it
		verifyOldValues(vallox, mqtt, cache)
		return
	}

	cached := cacheEntry{time: now, value: e}
	cache[e.Register] = cached

	publishValue(mqtt, cached.value)
}

func verifyOldValues(device *vallox.Vallox, mqtt mqttClient.Client, cache map[byte]cacheEntry) {
	// Speed is not automatically published by Vallox, so manually refresh the value
	now := time.Now()
	validTime := now.Add(time.Duration(-15) * time.Minute)
	if cached, ok := cache[vallox.FanSpeed]; ok && cached.time.Before(validTime) || !ok {
		device.Query(vallox.FanSpeed)
	}
}

func publishValue(mqtt mqttClient.Client, event vallox.Event) {

	if topic, ok := topicMap[event.Register]; ok {
		publish(mqtt, topic, fmt.Sprintf("%d", event.Value))
	}

	publish(mqtt, fmt.Sprintf("vallox/raw/%x", event.Register), fmt.Sprintf("%d", event.RawValue))
}

func publish(mqtt mqttClient.Client, topic string, msg interface{}) {
	logDebug.Printf("publishing to %s msg %s", msg, topic)

	t := mqtt.Publish(topic, 0, false, msg)
	go func() {
		_ = t.Wait()
		if t.Error() != nil {
			logError.Printf("publishing msg failed %v", t.Error())
		}
	}()
}

func discoveryMsg(uid string, name string, deviceClass string, stateTopic string) []byte {
	msg := make(map[string]interface{})
	msg["unique_id"] = uid
	msg["name"] = name
	dev := make(map[string]string)
	msg["device"] = dev
	dev["identifiers"] = "vallox"
	dev["manufacturer"] = "Vallox"
	dev["name"] = "Vallox Digit SE"
	dev["model"] = "Digit SE"
	if deviceClass != "" {
		msg["device_class"] = deviceClass
	}
	msg["expire_after"] = 1800
	msg["state_topic"] = stateTopic
	if strings.HasPrefix(uid, "vallox_temp") {
		msg["unit_of_measurement"] = "°C"
	}
	msg["state_class"] = "measurement"
	//msg["force_update"] = true

	jsonm, err := json.Marshal(msg)
	if err != nil {
		logError.Printf("cannot marshal json %v", err)
	}
	return jsonm
}

func announceMe(mqtt mqttClient.Client, cache map[byte]cacheEntry) {
	publishDiscovery(mqtt, "vallox_fan_speed", "Fan speed", "", topicFanSpeed)
	publishDiscovery(mqtt, "vallox_temp_incoming_outside", "Temperature incoming outside", "temperature", topicTempIncomingOutside)
	publishDiscovery(mqtt, "vallox_temp_incoming_insise", "Temperature incoming inside", "temperature", topicTempIncomingIside)
	publishDiscovery(mqtt, "vallox_temp_outgoing_inside", "Temperature outgoing inside", "temperature", topicTempOutgoingInside)
	publishDiscovery(mqtt, "vallox_temp_outgoing_outside", "Temperature outgoing outside", "temperature", topicTempOutgoingOutside)

	for reg, _ := range cache {
		announceRawData(mqtt, reg)
	}
}

func announceRawData(mqtt mqttClient.Client, register byte) {
	uid := fmt.Sprintf("vallox_raw_%x", register)
	name := fmt.Sprintf("Vallox raw %x", register)
	stateTopic := fmt.Sprintf("vallox/raw/%x", register)
	publishDiscovery(mqtt, uid, name, "", stateTopic)
}

func publishDiscovery(mqtt mqttClient.Client, uid string, name string, deviceClass string, stateTopic string) {
	discoveryTopic := fmt.Sprintf("homeassistant/sensor/%s/config", uid)
	msg := discoveryMsg(uid, name, deviceClass, stateTopic)
	publish(mqtt, discoveryTopic, msg)
}

func connectionLostHandler(client mqttClient.Client, err error) {
	options := client.OptionsReader()
	logError.Printf("MQTT connection to %s lost %v", options.Servers(), err)
}

func connectHandler(client mqttClient.Client) {
	options := client.OptionsReader()
	logInfo.Printf("MQTT connected to %s", options.Servers())
}

func reconnectHandler(client mqttClient.Client, options *mqttClient.ClientOptions) {
	logInfo.Printf("MQTT reconnecting to %s", options.Servers)
}

func initLogging() {
	writer := os.Stdout
	err := os.Stderr

	if config.Debug {
		logDebug = log.New(writer, "DEBUG ", log.Ldate|log.Ltime|log.Lmsgprefix)
	} else {
		logDebug = log.New(ioutil.Discard, "DEBUG ", 0)
	}
	logInfo = log.New(writer, "INFO  ", log.Ldate|log.Ltime|log.Lmsgprefix)
	logError = log.New(err, "ERROR ", log.Ldate|log.Ltime|log.Lmsgprefix)
}
