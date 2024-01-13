package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/tarm/serial"
)

type SensorValue struct {
	SensorLabel    string
	SensorType     string
	SensorInstance int
	SensorValue    float64
	SensorUnit     string
}

var mutSensors = map[string]string{
	"0x04": "Timing Advance Int",
	"0x06": "Timing Advance",
	"0x07": "Coolant Temp",
	"0x0c": "Fuel Trim Low (LTFT)",
	"0x0d": "Fuel Trim Mid (LTFT)",
	"0x0e": "Fuel Trim High (LTFT)",
	"0x0f": "Oxygen Feedback Trim (STFT)",
	"0x10": "Coolant Temp Scaled",
	"0x11": "MAF Air Temp",
	"0x12": "EGR Temperature",
	"0x13": "Front Oxygen Sensor",
	"0x14": "Battery Level",
	"0x15": "Barometer",
	"0x16": "ISC Steps",
	"0x17": "Throttle Position",
	"0x1a": "Air Flow Meter",
	"0x1c": "Engine Load",
	"0x1d": "Acceleration Enrichment",
	"0x1f": "ECU Load Previous load",
	"0x21": "Engine RPM",
	"0x24": "Target Idle RPM",
	"0x26": "Knock Sum",
	"0x29": "Injector Pulse Width",
	"0x2c": "Air Volume",
	"0x2f": "Speed",
	"0x30": "Knock Voltage",
	"0x31": "Volumetric Efficiency",
	"0x32": "Air/Fuel Ratio (Map)",
	"0x33": "Timing",
	"0x38": "Boost (MDP)",
	"0x39": "Fuel Tank Pressure",
	"0x3c": "Rear Oxygen Sensor #1",
	"0x3d": "Front Oxygen Sensor #2",
	"0x3e": "Rear Oxygen Sensor #2",
	"0x4a": "Purge Solenoid Duty Cycle",
	"0x80": "ECU ID Type",
	"0x82": "ECU ID Version",
	"0x85": "EGR Duty Cycle",
	"0x86": "Wastegate Duty Cycle",
	"0x96": "RAW MAF ADC value",
}

var mutSensorUnits = map[string]string{
	"0x04": "°",
	"0x06": "°",
	"0x07": "C",
	"0x0c": "%",
	"0x0d": "%",
	"0x0e": "%",
	"0x0f": "%",
	"0x10": "C",
	"0x11": "C",
	"0x12": "C",
	"0x13": "V",
	"0x14": "V",
	"0x15": "kPa",
	"0x16": "steps",
	"0x17": "%",
	"0x1a": "Hz",
	"0x1c": "%",
	"0x21": "RPM",
	"0x24": "RPM",
	"0x26": "knocks",
	"0x29": "ms",
	"0x2f": "KPH",
	"0x30": "V",
	"0x31": "V",
	"0x32": "AFR",
	"0x33": "°",
	"0x38": "PSI",
	"0x39": "PSI",
	"0x3c": "V",
	"0x3d": "V",
	"0x3e": "V",
	"0x4a": "%",
	"0x85": "%",
	"0x86": "%",
}

var imfdSensors = map[int]string{
	0:  "Wide-Band Air/Fuel",
	1:  "Exhaust Gas Temperature",
	2:  "Fluid Temperature",
	3:  "Vacuum",
	4:  "Boost",
	5:  "Air Intake Temperature",
	6:  "RPM",
	7:  "Vehicle Speed",
	8:  "Throttle Position",
	9:  "Engine Load",
	10: "Fuel Pressure",
	11: "Timing",
	12: "MAP",
	13: "MAF",
	14: "Short Term Fuel Trim",
	15: "Long Term Fuel Trim",
	16: "Narrow-Band Oxygen Sensor",
	17: "Fuel Level",
	18: "Volt Meter",
	19: "Knock",
	20: "Duty Cycle",
}

var sensorDataChannel = make(chan SensorValue)

func main() {
	if err := ui.Init(); err != nil {
		log.Fatal(err)
	}
	defer ui.Close()

	f, err := os.OpenFile("/tmp/dashboard.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}(f)
	log.SetOutput(f)

	// MAF Boost Pressure
	boost := widgets.NewGauge()
	boost.Percent = 0
	boost.Title = "Boost"
	boost.BarColor = ui.ColorRed
	boost.BorderStyle.Fg = ui.ColorWhite
	boost.TitleStyle.Fg = ui.ColorCyan

	// Throttle Position Sensor
	throttlePosition := widgets.NewGauge()
	throttlePosition.Title = "Throttle Position"
	throttlePosition.BarColor = ui.ColorRed
	throttlePosition.BorderStyle.Fg = ui.ColorWhite
	throttlePosition.LabelStyle.Fg = ui.ColorCyan

	// Engine RPM Gauge
	engineRPM := widgets.NewGauge()
	engineRPM.Percent = 0
	engineRPM.Title = "Engine RPM"
	engineRPM.BarColor = ui.ColorGreen
	engineRPM.BorderStyle.Fg = ui.ColorWhite
	engineRPM.LabelStyle.Fg = ui.ColorCyan

	// speed
	wheelSpeed := widgets.NewParagraph()
	wheelSpeed.Text = ""
	wheelSpeed.Title = "Speed"
	wheelSpeed.BorderStyle.Fg = ui.ColorBlack

	// coolantTemp
	coolantTemp := widgets.NewParagraph()
	coolantTemp.Text = "N/A °C"
	coolantTemp.Title = "Coolant Temp"
	coolantTemp.BorderStyle.Fg = ui.ColorBlack

	// intakeTemp
	intakeTemp := widgets.NewParagraph()
	intakeTemp.Text = "N/A °C"
	intakeTemp.Title = "Intake Air"
	intakeTemp.BorderStyle.Fg = ui.ColorBlack

	// engineTiming
	engineTiming := widgets.NewParagraph()
	engineTiming.Text = "N/A °"
	engineTiming.Title = "Engine Timing"
	engineTiming.BorderStyle.Fg = ui.ColorBlack

	// batteryVoltage
	batteryVoltage := widgets.NewParagraph()
	batteryVoltage.Text = "N/A V"
	batteryVoltage.Title = "Batt. Voltage"
	batteryVoltage.BorderStyle.Fg = ui.ColorBlack

	// knockCount
	knockCount := widgets.NewParagraph()
	knockCount.Text = "N/A"
	knockCount.Title = "Knock Count"
	knockCount.BorderStyle.Fg = ui.ColorBlack

	// Layout Grid
	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)

	grid.Set(
		ui.NewRow(1.0/8,
			ui.NewCol(1.0/6, engineTiming),
			ui.NewCol(1.0/6, wheelSpeed),
			ui.NewCol(1.0/6, knockCount),
			ui.NewCol(1.0/6, batteryVoltage),
			ui.NewCol(1.0/6, intakeTemp),
			ui.NewCol(1.0/6, coolantTemp),
		),
		ui.NewRow(2.0/8,
			ui.NewCol(1.0/1, throttlePosition),
		),
		ui.NewRow(2.0/8,
			ui.NewCol(1.0/1, engineRPM),
		),
		ui.NewRow(2.0/8,
			ui.NewCol(1.0/1, boost),
		),
	)

	ui.Render(grid)

	var wg sync.WaitGroup

	//wg.Add(1)
	//go func() {
	//	defer wg.Done()
	//	imfdStream()
	//}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		mutStream()
	}()

	// Event Loop
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(time.Millisecond * 100).C
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				//wg.Wait()
				return
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Clear()
				ui.Render(grid)
			}
		case payload := <-sensorDataChannel:
			log.Printf("Incoming Payload: |%s| -> %f [%s]", payload.SensorLabel, payload.SensorValue, payload.SensorUnit)
			switch payload.SensorLabel {
			case "/imfd-sensor/Boost":
				limit := 1.7
				boost.Percent = int((payload.SensorValue / limit) * 100)
				boost.Label = fmt.Sprintf("%f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(boost)
			case "/mut-sensor/Throttle Position":
				throttlePosition.Percent = int(payload.SensorValue)
				ui.Render(throttlePosition)
			case "/mut-sensor/Engine RPM":
				limit := 8000.0
				engineRPM.Percent = int((payload.SensorValue / limit) * 100)
				engineRPM.Label = fmt.Sprintf("%.0f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(engineRPM)
			case "/mut-sensor/Speed":
				wheelSpeed.Text = fmt.Sprintf("%.1f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(wheelSpeed)
			case "/mut-sensor/Coolant Temp":
				coolantTemp.Text = fmt.Sprintf("%.0f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(coolantTemp)
			case "/mut-sensor/Knock Sum":
				knockCount.Text = fmt.Sprintf("%.0f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(knockCount)
			case "/mut-sensor/MAF Air Temp":
				intakeTemp.Text = fmt.Sprintf("%.1f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(intakeTemp)
			case "/mut-sensor/Timing Advance":
				engineTiming.Text = fmt.Sprintf("%.1f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(engineTiming)
			case "/mut-sensor/Battery Level":
				batteryVoltage.Text = fmt.Sprintf("%.1f %s", payload.SensorValue, payload.SensorUnit)
				ui.Render(batteryVoltage)
			}
		case <-ticker:
			ui.Render(grid)
		}
	}
}

func mutStream() {

	l, err := net.Listen("unix", "/tmp/mut-stream.sock")
	if err != nil {
		log.Fatal("listen error:", err)
	}

	for {
		fd, err := l.Accept()
		if err != nil {
			log.Fatal("accept error:", err)
		}
		go mutReader(fd)
	}
}

func mutReader(c net.Conn) {
	for {
		reader := bufio.NewReader(c)
		_packet, err := reader.ReadBytes('\n')
		if err != nil {
			log.Fatal(err)
		}
		packet := string(_packet)
		packet = strings.TrimSuffix(packet, "\n")

		payload := strings.Split(packet, "|")
		sensor := payload[0]
		//log.Printf("MUT Input Packet: |%s| -> [%s] (%s|%f)", sensor, packet, payload[0], payload[1])
		value, err := strconv.ParseFloat(payload[1], 64)

		if len(packet) < 8 {
			continue
		}

		if sensor == "" {
			continue
		}

		decodedData := mutSensorDecode(sensor, value)
		sensorDataChannel <- decodedData

		if err != nil {
			log.Fatal("Error: ", err)
		}
	}
}

func imfdStream() {
	log.Println("IMFD thread started")
	c := &serial.Config{Name: "/dev/ttyS0", Baud: 19200}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}
	for {
		reader := bufio.NewReader(s)
		reply, err := reader.ReadBytes('@')
		if err != nil {
			log.Fatal(err)
		}
		if len(reply) >= 7 {
			// Start at an offset (to ignore the start bit)
			i := 1
			// Finish one from the end (to ignore the stop bit)
			to := len(reply) - 2

			for i < to {
				last := i + 5

				// Pluck out the Sensor Data
				sensorPacket := reply[i:last]

				// Grab the Sensor ID
				addr := (sensorPacket[0] << 6) | sensorPacket[1]
				sensorType := int(addr)

				// The sensor instance ID (If you have multiple sensors)
				instance := int(sensorPacket[2])

				// And the sensor value
				sensorValueB := (sensorPacket[3] << 6) | sensorPacket[4]
				sensorValue, err := strconv.ParseFloat(strconv.Itoa(int(sensorValueB)), 64)
				if err != nil {
					log.Fatal(err)
				}

				// Decode the packet back into a struct
				decodedData := imfdSensorDecode(sensorType, sensorValue, instance)

				// Send the decoded data to the channel
				sensorDataChannel <- decodedData

				log.Printf("Event Fired: %s", decodedData.SensorLabel)

				// Increment the for loop, so we can get to the next sensor in the chain
				i = last
			}
		} else {
			fmt.Printf("Malformed frame detected (runt): %s\n", reply)
		}
	}
}

func mutSensorDecode(sensorType string, sensorValue float64) SensorValue {
	var result float64
	sensorName := mutSensors[sensorType]
	sensorUnit := mutSensorUnits[sensorType]
	sensorLabel := fmt.Sprintf("/mut-sensor/%s", sensorName)

	switch sensorType {
	case "0x04":
		result = sensorValue - 20
	case "0x06":
		result = sensorValue - 20
	case "0x07":
		result = sensorValue - 40
	case "0x0c":
		result = (sensorValue - 128) / 5
	case "0x0d":
		result = (sensorValue - 128) / 5
	case "0x0e":
		result = (sensorValue - 128) / 5
	case "0x0f":
		result = (sensorValue - 128) / 5
	case "0x10":
		result = sensorValue - 40
	case "0x11":
		result = sensorValue - 40
	case "0x12":
		result = (-2.7*sensorValue + 597.7) * 0.556
	case "0x13":
		result = 0.01952 * sensorValue
	case "0x14":
		result = 0.07333 * sensorValue
	case "0x15":
		result = 0.49 * sensorValue
	case "0x16":
		result = 100 * sensorValue / 120
	case "0x17":
		result = sensorValue * 100 / 255
	case "0x1a":
		result = 6.25 * sensorValue
	case "0x1c":
		result = 5 * sensorValue / 8
	case "0x1d":
		result = 200 * sensorValue / 255
	case "0x1f":
		result = 5 * sensorValue / 8
	case "0x21":
		result = 31.25 * sensorValue
	case "0x24":
		result = 7.8 * sensorValue
	case "0x29":
		result = sensorValue / 1000
	case "0x2f":
		result = 2 * sensorValue
	case "0x30":
		result = 0.0195 * sensorValue
	case "0x31":
		result = 0.0195 * sensorValue
	case "0x32":
		result = (14.7 * 128) / sensorValue
	case "0x33":
		result = sensorValue - 20
	case "0x38":
		result = 0.19348 * sensorValue
	case "0x3c":
		result = 0.01952 * sensorValue
	case "0x3d":
		result = 0.01952 * sensorValue
	case "0x3e":
		result = 0.01952 * sensorValue
	case "0x4a":
		result = sensorValue * 100 / 255
	case "0x85":
		result = sensorValue / 1.28
	case "0x86":
		result = sensorValue / 2
	}
	return SensorValue{sensorLabel, sensorName, 0, result, sensorUnit}

}

func imfdSensorDecode(sensorType int, sensorValue float64, instanceId int) SensorValue {
	var result float64
	var unit string

	sensorName := imfdSensors[sensorType]
	sensorLabel := fmt.Sprintf("/imfd-sensor/%s", sensorName)

	switch sensorType {
	case 0:
		result = (sensorValue/3.75 + 68) / 100
		unit = "Lambda"
	case 1:
		result = sensorValue
		unit = "°C"
	case 2:
		result = sensorValue
		unit = "°C"
	case 3:
		result = sensorValue*2.23 + 760.4
		unit = "mm/Hg"
	case 4:
		result = (sensorValue / 329.48) * 0.0689476
		unit = "Bar"
	case 5:
		result = sensorValue
		unit = "°C"
	case 6:
		result = sensorValue * 19.55
		unit = "RPM"
	case 7:
		result = sensorValue / 3.97
		unit = "KPH"
	case 8:
		result = sensorValue
		unit = "%"
	case 9:
		result = sensorValue
		unit = "%"
	case 10:
		result = sensorValue / 74.22
		unit = "Bar"
	case 11:
		result = sensorValue - 64
		unit = "°"
	case 12:
		result = sensorValue
		unit = "kPa"
	case 13:
		result = sensorValue
		unit = "g/s"
	case 14:
		result = sensorValue - 100
		unit = "%"
	case 15:
		result = sensorValue - 100
		unit = "%"
	case 16:
		result = sensorValue
		unit = "%"
	case 17:
		result = sensorValue
		unit = "%"
	case 18:
		result = sensorValue / 51.15
		unit = "V"
	case 19:
		result = sensorValue / 204.6
		unit = "V"
	case 20:
		result = sensorValue / 10.23
		unit = "+ Duty"
	}

	return SensorValue{sensorLabel, imfdSensors[sensorType], instanceId, result, unit}
}
