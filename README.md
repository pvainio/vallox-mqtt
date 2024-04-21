# Vallox RS485 MQTT gateway for Home Assistant

## Overview

This rs485 mqtt gateway can be used to publish events from Vallox rs485 serial bus to mqtt and send commands to Vallox devices via mqtt.

It supports Home Assistant MQTT Discovery but can also be used without Home Assistant.

Only requirement is MQTT Broker to connect to.

## Supported features

Supports following features:
- Home Assistant MQTT discovery, published device automatically to Home Assistant
- Published regular intervals:
  * Ventilation fan speed
  * Outside temperature (sensor.temperature_incoming_outside)
  * Incoming temperature (sensor.temperature_incoming_inside)
  * Inside temperature (sensor.temperature_outgoing_inside)
  * Exhaust temperature (sensor.temperature_outgoing_outside)
- Change ventilation speed

## Supported devices

Use at your own risk.

Only tested with:
- Vallox Digit SE model 3500 SE made in 2001 (one with old led panel, no lcd panel)

Might work with other Vallox devices with rs485 bus.  There probably are some differences between different devices.  If there are those probably are easy to adapt to.

The application itself has been tested running on Raspberry Pi 3, but probably works just fine with Raspberry Zero or anything running linux.

To compile for Raspberry PI: env GOOS=linux GOARCH=arm go build -o vallox_mqtt

Quality RS485 adapter should be used, there can be strange problems with low quality ones.

## Example usecase

Can be used to monitor and command Vallox ventilation device with Home Assistant.  Raspberry Pi with properer usb to rs485 adapter can act as a gateway between Vallox and MQTT (and Home Assistant).  Automation can be built to increase the speed in case of high CO2 or high humidity even if the Vallox device is not installed with co2 and humidity sensors.

### Home Assistant Card screenshots

Speed select and graph:
![outdoor temp grap](https://github.com/pvainio/vallox-mqtt/blob/main/img/ha-graph-speed.png?raw=true)

Temperature graph:
![outdoor temp grap](https://github.com/pvainio/vallox-mqtt/blob/main/img/ha-graph-temp.png?raw=true)

Outdoor temperature graph:
![outdoor temp grap](https://github.com/pvainio/vallox-mqtt/blob/main/img/ha-graph-outtemp.png?raw=true)

## Configuration

Application is configure with environment variables

| variable        | required | default | description |
|-----------------|:--------:|---------|-------------|
| SERIAL_DEVICE   |    x     |         | serial device, for example /dev/ttyUSB0 |
| MQTT_URL        |    x     |         | mqtt url, for example tcp://10.1.2.3:8883 |
| MQTT_USER       |          |         | mqtt username |
| MQTT_PASSWORD   |          |         | mqtt password |
| MQTT_CLIENT_ID  |          | same as DEVICE_ID  | mqtt client id |
| DEVICE_ID       |          | vallox  | id for homeassistant device and also act as mqtt base topic |
| DEVICE_NAME     |          | Vallox  | Home assistant device name |
| DEBUG           |          | false   | enable debug output, true/false |
| ENABLE_WRITE    |          | false   | enable sending commands/writing to bus, true/false |
| SPEED_MIN       |          | 1       | minimum speed for the device, between 1-8.  Used for HA discovery to have correct min value in UI |
| ENABLE_RAW      |          | false   | enable sending raw events to mqtt, otherwise only known changes are sent |
| OBJECT_ID       |          | true    | Send object_id with HA Auto Discovery for HA entity names |

## Multiple Devices

Running multiple devices is supported (although not tested).  Currently this requires
running own process for each device.  DEVICE_ID and DEVICE_NAME shoud be set uniquely for each device, like DEVICE_ID=vallox1, DEVICE_NAME="Vallox 1" for one device and DEVICE_ID=vallox2, DEVICE_NAME="Vallox 2" for other device.

## Usage

For example with following script
```sh
#!/bin/sh

# Change to your real rs485 device
export SERIAL_DEVICE=/dev/ttyUSB0
# Change to your real mqtt url
export MQTT_URL=tcp://localhost:8883
# Set device id and name, in case of multiple devices
export DEVICE_ID=valloxupstairs
export DEVICE_NAME="Vallox Upstairs"

./vallox-mqtt
```

## MQTT Topics used

With default configuration:
- homeassistant/status subscribe to HA status changes
- vallox/fan/set subscribe to fan speed commands
- vallox/fan/speed publish fan speeds
- vallox/temperature_incoming_outside Outdoor temperature
- vallox/temperature_incoming_inside Incoming temperature
- vallox/temperature_outgoing_inside Inside temperature
- vallox/temperature_outgoing_outside Exhaust temperature
- vallox/raw/# Raw register value changes (if raw values are enabled)

If DEVICE_ID is specified it is used as mqtt base topic, for example if DEVICE_ID=vallox1 then topics would be:
- vallox1/fan/set subscribe to fan speed commands
- vallox1/fan/speed publish fan speeds
- vallox1/temperature_incoming_outside Outdoor temperature
- vallox1/temperature_incoming_inside Incoming temperature
- vallox1/temperature_outgoing_inside Inside temperature
- vallox1/temperature_outgoing_outside Exhaust temperature
- vallox1/raw/# Raw register value changes (if raw values are enabled)

## Home Assistant sensors

If mqtt auto discovery is used and OBJECT_ID is true (default) Home Assistant sensors are created based on DEVICE_ID like:
- sensor.vallox_fan_speed
- select.vallox_fan_select
- sensor.vallox_temp_incoming_outside
- sensor.vallox_temp_incoming_insise
- sensor.vallox_temp_outgoing_inside
- sensor.vallox_temp_outgoing_outside

Without OBJECT_ID sensor ids are automatically created by HA based on sensor names
