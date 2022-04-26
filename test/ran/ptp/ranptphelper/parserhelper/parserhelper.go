package ranptpparserhelper

import (
	"strings"
)

type Log struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

type EventMsg struct {
	ID              string    `json:"id"`
	EventType       string    `json:"type"`
	Source          string    `json:"source"`
	DataContentType string    `json:"dataContentType"`
	Time            string    `json:"time"`
	Data            eventData `json:"data"`
}

type eventData struct {
	Version string   `json:"version"`
	Values  []values `json:"values"`
}

type values struct {
	Resource  string `json:"resource"`
	DataType  string `json:"dataType"`
	ValueType string `json:"valueType"`
	Value     string `json:"value"`
}

// LogStrToLogStrct a single log as a string from the pod's logger and parsers it into a "Log" structure
// and returns that structure.
func LogStrToLogStrct(log string) Log {
	var logDetails Log

	timeStartIndx := strings.Index(log, "\"") + 1
	timeEndIndx := strings.Index(log, " ") - 1
	logDetails.Time = log[timeStartIndx:timeEndIndx]

	log = log[timeEndIndx+2:]
	levelStartIndx := strings.Index(log, "=") + 1
	levelEndIndx := strings.Index(log, " ")
	logDetails.Level = log[levelStartIndx:levelEndIndx]

	log = log[levelEndIndx+2:]
	msgStartIndx := strings.Index(log, "=") + 2
	logDetails.Msg = log[msgStartIndx : len(log)-1]

	return logDetails
}

// EventMsgParser gets a single PTP event as a string from the log message and parsers it into a "EventMsg" structure
// and returns that structure.
func EventMsgParser(strFormat string) EventMsg {
	var eventMassage EventMsg

	idStart := strFormat[strings.Index(strFormat, "id")+8:]
	eventTypeStart := idStart[strings.Index(idStart, "type")+10:]
	sourceStart := eventTypeStart[strings.Index(eventTypeStart, "source")+12:]
	dataContentTypeStart := sourceStart[strings.Index(sourceStart, "dataContentType")+21:]
	eventTimeStart := dataContentTypeStart[strings.Index(dataContentTypeStart, ":")+4:]
	eventDataStart := eventTimeStart[strings.Index(eventTimeStart, "data")+8:]

	eventMassage.ID = extractDetail(idStart)
	eventMassage.EventType = extractDetail(eventTypeStart)
	eventMassage.Source = extractDetail(sourceStart)
	eventMassage.DataContentType = extractDetail(dataContentTypeStart)
	eventMassage.Time = extractDetail(eventTimeStart)

	eventMassage.Data = eventDataParser(eventDataStart)

	return eventMassage
}

// eventDataParser gets the "Data" from the PTP event as a string and parsers it into a "eventData" structure
// and returns that structure.
func eventDataParser(eventDataStr string) eventData {
	var eventDataFull eventData

	versionStart := eventDataStr[strings.Index(eventDataStr, "version")+13:]
	valuesArray := versionStart[strings.Index(versionStart, "[") : strings.Index(versionStart, "]")+1]

	eventDataFull.Version = extractDetail(versionStart)
	eventDataFull.Values = eventDataValuesParser(valuesArray)

	return eventDataFull
}

// eventDataValuesParser gets the "Values" from the PTP event data as a string and parsers it into a "values" structure
// and returns an array of "values" structure.
func eventDataValuesParser(eventDataValuesStr string) []values {
	var eventDataValuesFull []values

	str := eventDataValuesStr

	for strings.Contains(str, "resource") {
		resourcesStart := str[strings.Index(str, "resource")+15:]
		dataTypeStart := resourcesStart[strings.Index(resourcesStart, "dataType")+14:]
		valueTypeStart := dataTypeStart[strings.Index(dataTypeStart, "valueType")+15:]
		valueStart := valueTypeStart[strings.Index(valueTypeStart, "value")+11:]

		eventDataValuesFull = append(eventDataValuesFull,
			values{extractDetail(resourcesStart), extractDetail(dataTypeStart),
				extractDetail(valueTypeStart), extractDetail(valueStart)})
		str = valueStart
	}

	return eventDataValuesFull
}

// extractDetail gets a string with the "\"" characters
// and returns a substring from the begging of the given string until the first ""\"" characters.
func extractDetail(start string) string {
	return start[:strings.Index(start, "\"")-1]
}
