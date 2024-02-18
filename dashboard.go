package main

import (
	"bufio"
	"container/heap"
	"encoding/binary"
	"fmt"
	"github.com/ziutek/ftdi"
	"go.bug.st/serial"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type SensorValue struct {
	SensorLabel    string
	SensorType     string
	SensorInstance int
	SensorValue    float64
	SensorUnit     string
}

type mutResponse struct {
	sensorId uint16
	value    uint16
}

type sensorQueue []*sensorRequest

type sensorRequest struct {
	sensorId uint16
}

type mutSensor struct {
	name               string
	unit               string
	conversionFunction func(float64) float64
	priority           string
}

var mutSensors = map[uint16]mutSensor{
	0x0004: {
		"Timing Advance Int",
		"°",
		func(sensorValue float64) float64 { return sensorValue - 20 },
		"medium",
	},
	0x0006: {
		"Timing Advance",
		"°",
		func(sensorValue float64) float64 { return sensorValue - 20 },
		"medium",
	},
	0x0007: {
		"Coolant Temp",
		"C",
		func(sensorValue float64) float64 { return sensorValue - 40 },
		"medium",
	},
	0x000c: {
		"Fuel Trim Low (LTFT)",
		"%",
		func(sensorValue float64) float64 { return (sensorValue - 128) / 5 },
		"none",
	},
	0x000d: {
		"Fuel Trim Mid (LTFT)",
		"%",
		func(sensorValue float64) float64 { return (sensorValue - 128) / 5 },
		"none",
	},
	0x000e: {
		"Fuel Trim High (LTFT)",
		"%",
		func(sensorValue float64) float64 { return (sensorValue - 128) / 5 },
		"none",
	},
	0x000f: {
		"Oxygen Feedback Trim (STFT)",
		"%",
		func(sensorValue float64) float64 { return (sensorValue - 128) / 5 },
		"none",
	},
	0x0010: {
		"Coolant Temp Scaled",
		"C",
		func(sensorValue float64) float64 { return sensorValue - 40 },
		"medium",
	},
	0x0011: {
		"MAF Air Temp",
		"C",
		func(sensorValue float64) float64 { return sensorValue - 40 },
		"low",
	},
	0x0012: {
		"EGR Temperature",
		"C",
		func(sensorValue float64) float64 { return (-2.7*sensorValue + 597.7) * 0.556 },
		"medium",
	},
	0x0013: {
		"Front Oxygen Sensor",
		"V",
		func(sensorValue float64) float64 { return 0.01952 * sensorValue },
		"none",
	},
	0x0014: {
		"Battery Level",
		"V",
		func(sensorValue float64) float64 { return 0.07333 * sensorValue },
		"medium",
	},
	0x0015: {
		"Barometer",
		"kPa",
		func(sensorValue float64) float64 { return 0.49 * sensorValue },
		"none",
	},
	0x0016: {
		"ISC Steps",
		"steps",
		func(sensorValue float64) float64 { return 100 * sensorValue / 120 },
		"low",
	},
	0x0017: {
		"Throttle Position",
		"%",
		func(sensorValue float64) float64 { return sensorValue * 100 / 255 },
		"high",
	},
	0x001a: {
		"Air Flow Meter",
		"Hz",
		func(sensorValue float64) float64 { return 6.25 * sensorValue },
		"medium",
	},
	0x001c: {
		"Engine Load",
		"%",
		func(sensorValue float64) float64 { return 5 * sensorValue / 8 },
		"medium",
	},
	0x001d: {
		"Acceleration Enrichment",
		"",
		func(sensorValue float64) float64 { return 200 * sensorValue / 255 },
		"none",
	},
	0x001f: {
		"ECU Load Previous load",
		"%",
		func(sensorValue float64) float64 { return 5 * sensorValue / 8 },
		"none",
	},
	0x0021: {
		"Engine RPM",
		"RPM",
		func(sensorValue float64) float64 { return 31.25 * sensorValue },
		"high",
	},
	0x0024: {
		"Target Idle RPM",
		"RPM",
		func(sensorValue float64) float64 { return 7.8 * sensorValue },
		"low",
	},
	0x0026: {
		"Knock Sum",
		"knocks",
		func(sensorValue float64) float64 { return sensorValue },
		"high",
	},
	0x0029: {
		"Injector Pulse Width",
		"ms",
		func(sensorValue float64) float64 { return sensorValue / 1000 },
		"medium",
	},
	0x002c: {
		"Air Volume",
		"",
		func(sensorValue float64) float64 { return sensorValue },
		"low",
	},
	0x002f: {
		"Speed",
		"km/h",
		func(sensorValue float64) float64 { return 2 * sensorValue },
		"high",
	},
	0x0030: {
		"Knock Voltage",
		"V",
		func(sensorValue float64) float64 { return 0.0195 * sensorValue },
		"none",
	},
	0x0031: {
		"Volumetric Efficiency",
		"V",
		func(sensorValue float64) float64 { return 0.0195 * sensorValue },
		"none",
	},
	0x0032: {
		"Air/Fuel Ratio (Map)",
		"AFR",
		func(sensorValue float64) float64 { return (14.7 * 128) / sensorValue },
		"medium",
	},
	0x0033: {
		"Timing",
		"°",
		func(sensorValue float64) float64 { return sensorValue - 20 },
		"high",
	},
	0x0038: {
		"Boost (MDP)",
		"PSI",
		func(sensorValue float64) float64 { return 0.19348 * sensorValue },
		"low",
	},
	0x0039: {
		"Fuel Tank Pressure",
		"PSI",
		func(sensorValue float64) float64 { return sensorValue },
		"none",
	},
	0x003c: {
		"Rear Oxygen Sensor #1",
		"V",
		func(sensorValue float64) float64 { return 0.01952 * sensorValue },
		"none",
	},
	0x003d: {
		"Front Oxygen Sensor #2",
		"V",
		func(sensorValue float64) float64 { return 0.01952 * sensorValue },
		"none",
	},
	0x003e: {
		"Rear Oxygen Sensor #2",
		"V",
		func(sensorValue float64) float64 { return 0.01952 * sensorValue },
		"none",
	},
	0x004a: {
		"Purge Solenoid Duty Cycle",
		"%",
		func(sensorValue float64) float64 { return sensorValue * 100 / 255 },
		"none",
	},
	0x0080: {
		"ECU ID Type",
		"",
		func(sensorValue float64) float64 { return sensorValue },
		"none",
	},
	0x0082: {
		"ECU ID Version",
		"",
		func(sensorValue float64) float64 { return sensorValue },
		"none",
	},
	0x0085: {
		"EGR Duty Cycle",
		"",
		func(sensorValue float64) float64 { return sensorValue / 1.28 },
		"low",
	},
	0x0086: {
		"Wastegate Duty Cycle",
		"%",
		func(sensorValue float64) float64 { return sensorValue / 2 },
		"low",
	},
	0x0096: {
		"RAW MAF ADC value",
		"V",
		func(sensorValue float64) float64 { return sensorValue },
		"none",
	},
}

var sensorDataChannel = make(chan SensorValue)

var mutResponses = make(chan mutResponse)

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

	var wg sync.WaitGroup

	//wg.Add(1)
	//go func() {
	//	defer wg.Done()
	//	imfdStream()
	//}()

	fmt.Println("Starting MUT Streamer")
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutStream()
	}()

	fmt.Println("Starting MUT Reader")
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutReader()
	}()

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

	// Event Loop
	uiEvents := ui.PollEvents()
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
			log.Printf("[UI Loop] Incoming Payload: |%s/%s| -> %f [%s]", payload.SensorType, payload.SensorLabel, payload.SensorValue, payload.SensorUnit)
			fullLabel := fmt.Sprintf("/%s/%s", payload.SensorType, payload.SensorLabel)
			switch fullLabel {
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
		}
	}
}

func logError(err error, andPanic bool) {
	if err != nil {
		if andPanic == true {
			panic(err)
		} else {
			log.Fatal(err)
		}
	}
}

// Sensor Queue

func (pq sensorQueue) Len() int { return len(pq) }

func (pq sensorQueue) Less(i, j int) bool {
	// Compare based on index instead of priority
	return i < j
}

func (pq sensorQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *sensorQueue) Push(x interface{}) {
	item := x.(*sensorRequest)
	*pq = append(*pq, item)
}

func (pq *sensorQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// MUT sensors
func mutSerialInit() *ftdi.Device {
	// define a 2 byte buffer to store the response from the ECU
	var buf [2]byte

	// Steps:
	// Open the Serial Driver
	// Reset the device
	// Purge the RX and TX buffers on the device
	// Set the baud rate to 15625 baud
	// Set 8bits, 1 stop bit, no parity
	// Disable flow control
	// Set Latency timers
	// Initialize the MCU by sending 0x00 at 5 baud
	// Send 0xFF and 0xFE to get the ECU ID
	// ???
	// Profit

	serialDevice, error := ftdi.OpenFirst(0x0403, 0x6001, ftdi.ChannelAny)
	logError(error, true)

	// Reset the device
	serialDevice.Reset()
	serialDevice.PurgeBuffers()
	serialDevice.SetBaudrate(15625)
	serialDevice.SetLineProperties2(ftdi.DataBits8, ftdi.StopBits1, ftdi.ParityNone, ftdi.BreakOff)
	serialDevice.SetFlowControl(ftdi.FlowCtrlDisable)
	serialDevice.SetLatencyTimer(1)

	// MUT requires 0x00 to be sent to the ECU to start the stream
	// but at 5 baud. Instead, we can simply hold break
	// to start the stream, which is basically the same thing.
	// The original C code did the following between toggling break:
	// usleep(1800 * 1000)
	serialDevice.SetLineProperties2(ftdi.DataBits8, ftdi.StopBits1, ftdi.ParityNone, ftdi.BreakOn)
	time.Sleep(1800 * time.Millisecond)
	serialDevice.SetLineProperties2(ftdi.DataBits8, ftdi.StopBits1, ftdi.ParityNone, ftdi.BreakOff)

	// To make sure we have communication with the ECU,
	// We can ask for the ECU ID, this is done by sending 0xFF then 0xFE
	// The ECU should respond with two bytes, which combined will be the ECU ID

	// define the array of the bytes to send to the ECU
	bytes := []byte{0xFF, 0xFE}

	// Loop through the bytes and send them to the ECU
	for _, b := range bytes {
		serialDevice.Write([]byte{b})
		bytes, err := serialDevice.Read(buf[:])
		logError(err, false)

		if bytes < 1 || bytes > 2 {
			log.Fatal("ECU Initialization Failed: Expected between 1..2 byte(s), got ", bytes)
		}
	}

	log.Printf("ECU ID: %s", buf)

	return serialDevice
}

// This is the main loop for the MUT stream
// It is responsible for defining what sensors need to be checked
// and then checking them at a regular interval
func mutStream() {

	// Call the mutSerialInit function to initialize the serial device,
	// this should get the ECU ready to talk to us
	ecuSerialDevice := mutSerialInit()

	// Define the sensor queues
	highPriorityQueue := make(sensorQueue, 0)
	mediumPriorityQueue := make(sensorQueue, 0)
	lowPriorityQueue := make(sensorQueue, 0)

	// Define the temporary queues
	highPriorityTempQueue := make(sensorQueue, 0)
	mediumPriorityTempQueue := make(sensorQueue, 0)
	lowPriorityTempQueue := make(sensorQueue, 0)

	// Initialize the queue
	heap.Init(&highPriorityQueue)
	heap.Init(&mediumPriorityQueue)
	heap.Init(&lowPriorityQueue)

	// Loop over the mutSensors, pushing them onto the queue
	// if the priority is -99, skip them.
	// Loop over the mutSensors, pushing them onto the appropriate queue
	// if the priority is 'none', skip them.
	for sensorId, sensor := range mutSensors {
		if sensor.priority == "none" {
			continue
		}
		switch sensor.priority {
		case "high":
			heap.Push(&highPriorityQueue, &sensorRequest{sensorId: sensorId})
		case "medium":
			heap.Push(&mediumPriorityQueue, &sensorRequest{sensorId: sensorId})
		case "low":
			heap.Push(&lowPriorityQueue, &sensorRequest{sensorId: sensorId})
		}
	}

	// Define the tickers
	highPriorityTicker := time.NewTicker(time.Millisecond * 20)
	mediumPriorityTicker := time.NewTicker(time.Millisecond * 40)
	lowPriorityTicker := time.NewTicker(time.Millisecond * 100)

	defer highPriorityTicker.Stop()
	defer mediumPriorityTicker.Stop()
	defer lowPriorityTicker.Stop()

	for {
		select {

		case <-highPriorityTicker.C:
			if highPriorityQueue.Len() == 0 {
				// If the main queue is empty, push all sensors from the temporary queue back to the main queue
				for highPriorityTempQueue.Len() > 0 {
					sensorRequest := heap.Pop(&highPriorityTempQueue).(*sensorRequest)
					heap.Push(&highPriorityQueue, sensorRequest)
				}
				continue
			}
			// Pop the first item off the high priority queue
			sensorRequest := heap.Pop(&highPriorityQueue).(*sensorRequest)
			processSensorRequest(ecuSerialDevice, &highPriorityQueue, &highPriorityTempQueue, sensorRequest)
		case <-mediumPriorityTicker.C:
			if mediumPriorityQueue.Len() == 0 {
				// If the main queue is empty, push all sensors from the temporary queue back to the main queue
				for mediumPriorityTempQueue.Len() > 0 {
					sensorRequest := heap.Pop(&mediumPriorityTempQueue).(*sensorRequest)
					heap.Push(&mediumPriorityQueue, sensorRequest)
				}
				continue
			}
			// Pop the first item off the medium priority queue
			sensorRequest := heap.Pop(&mediumPriorityQueue).(*sensorRequest)
			processSensorRequest(ecuSerialDevice, &mediumPriorityQueue, &mediumPriorityTempQueue, sensorRequest)
		case <-lowPriorityTicker.C:
			if lowPriorityQueue.Len() == 0 {
				for lowPriorityTempQueue.Len() > 0 {
					sensorRequest := heap.Pop(&lowPriorityTempQueue).(*sensorRequest)
					heap.Push(&lowPriorityQueue, sensorRequest)
				}
				continue // Nothing to do
			}
			// Pop the first item off the low priority queue
			sensorRequest := heap.Pop(&lowPriorityQueue).(*sensorRequest)
			processSensorRequest(ecuSerialDevice, &lowPriorityQueue, &lowPriorityTempQueue, sensorRequest)
		}
	}
}

func processSensorRequest(ecuSerialDevice *ftdi.Device, queue *sensorQueue, tempQueue *sensorQueue, sensorRequest *sensorRequest) {
	// Send the requested sensor ID (byte) to the ECU and store the response
	response := mutWriter(*ecuSerialDevice, sensorRequest.sensorId)
	log.Println("Response: ", response)

	// Send the response to the mutResponses channel
	mutResponses <- mutResponse{sensorRequest.sensorId, response}

	// Push the sensor request back to the temporary queue instead of the main queue
	heap.Push(tempQueue, sensorRequest)
	log.Println("Queue: ", *queue)
}

// This function fires when a response is received from the ECU from the mutStream
// It is responsible for decoding the response and sending it to the sensorDataChannel
// for the UI to display
func mutReader() {
	for payload := range mutResponses {

		// Decode the response into a struct
		decodedData := mutSensorDecode(
			payload.sensorId,
			float64(payload.value),
		)
		log.Printf("[MUT Reader] Decoded Payload: |%s/%s| -> %f", decodedData.SensorType, decodedData.SensorLabel, decodedData.SensorValue)

		// Send the decoded data to the channel
		sensorDataChannel <- decodedData
	}
}

// Request a sensor value from the ECU and return the response
func mutWriter(ftdiDevice ftdi.Device, sensorId uint16) uint16 {
	log.Printf("Sending MUT Request for Sensor: %s", mutSensors[sensorId].name)

	// initialise the buffer with the sensor ID
	var outputBuffer = make([]byte, 2)
	binary.BigEndian.PutUint16(outputBuffer, sensorId)

	// write the sensor ID to the ECU then
	// read the response into the 'bytes' variable
	ftdiDevice.Write(outputBuffer)
	bytes, err := ftdiDevice.Read(outputBuffer)

	// log any errors
	logError(err, false)

	// we're expecting a value of at least 1 byte, check that we got one
	if bytes < 1 {
		log.Fatal("Expected more than 0 bytes got ", bytes)
	}

	// return the response from the ECU
	return uint16(bytes)
}

// Decode the sensor response from the ECU into a struct, and perform any necessary conversions
// then return it to the mutReader
func mutSensorDecode(sensorType uint16, sensorValue float64) SensorValue {
	sensor := mutSensors[sensorType]
	result := sensor.conversionFunction(sensorValue)
	return SensorValue{sensor.name, "mut-sensor", 0, result, sensor.unit}
}

// IMFD sensors
type imfdSensor struct {
	name               string
	unit               string
	conversionFunction func(float64) float64
}

var imfdSensors = map[int]imfdSensor{
	0: {
		"Wide-Band Air/Fuel",
		"Lambda",
		func(sensorValue float64) float64 { return (sensorValue/3.75 + 68) / 100 },
	},
	1: {
		"Exhaust Gas Temperature",
		"°C",
		func(sensorValue float64) float64 { return sensorValue },
	},
	2: {
		"Fluid Temperature",
		"°C",
		func(sensorValue float64) float64 { return sensorValue },
	},
	3: {
		"Vacuum",
		"mm/Hg",
		func(sensorValue float64) float64 { return sensorValue*2.23 + 760.4 },
	},
	4: {
		"Boost",
		"Bar",
		func(sensorValue float64) float64 { return (sensorValue / 329.48) * 0.0689476 },
	},
	5: {
		"Air Intake Temperature",
		"°C",
		func(sensorValue float64) float64 { return sensorValue },
	},
	6: {
		"RPM",
		"RPM",
		func(sensorValue float64) float64 { return sensorValue * 19.55 },
	},
	7: {
		"Vehicle Speed",
		"km/h",
		func(sensorValue float64) float64 { return sensorValue / 3.97 },
	},
	8: {
		"Throttle Position",
		"%",
		func(sensorValue float64) float64 { return sensorValue },
	},
	9: {
		"Engine Load",
		"%",
		func(sensorValue float64) float64 { return sensorValue },
	},
	10: {
		"Fuel Pressure",
		"Bar",
		func(sensorValue float64) float64 { return sensorValue / 74.22 },
	},
	11: {
		"Timing",
		"°",
		func(sensorValue float64) float64 { return sensorValue - 64 },
	},
	12: {
		"MAP",
		"kPa",
		func(sensorValue float64) float64 { return sensorValue },
	},
	13: {
		"MAF",
		"g/s",
		func(sensorValue float64) float64 { return sensorValue },
	},
	14: {
		"Short Term Fuel Trim",
		"%",
		func(sensorValue float64) float64 { return sensorValue - 100 },
	},
	15: {
		"Long Term Fuel Trim",
		"%",
		func(sensorValue float64) float64 { return sensorValue - 100 },
	},
	16: {
		"Narrow-Band Oxygen Sensor",
		"%",
		func(sensorValue float64) float64 { return sensorValue },
	},
	17: {
		"Fuel Level",
		"%",
		func(sensorValue float64) float64 { return sensorValue },
	},
	18: {
		"Volt Meter",
		"V",
		func(sensorValue float64) float64 { return sensorValue / 51.15 },
	},
	19: {
		"Knock",
		"V",
		func(sensorValue float64) float64 { return sensorValue / 204.6 },
	},
	20: {
		"Duty Cycle",
		"+ Duty",
		func(sensorValue float64) float64 { return sensorValue / 10.23 },
	},
}

func imfdStream() {
	log.Println("IMFD thread started")
	serialMode := &serial.Mode{
		BaudRate: 19200,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}
	s, err := serial.Open("/dev/ttyS0", serialMode)
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

// Decode the sensor response from the IMFD into a struct, and perform any necessary conversions
func imfdSensorDecode(sensorType int, sensorValue float64, instanceId int) SensorValue {
	var result float64
	var unit string

	sensorName := imfdSensors[sensorType].name
	sensorLabel := fmt.Sprintf("/imfd-sensor/%s", sensorName)

	return SensorValue{sensorLabel, imfdSensors[sensorType].name, instanceId, result, unit}
}
