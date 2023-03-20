package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"

	"strings"
)

// EventMsgParser gets a single PTP event as a string from the log message and parsers it into a "EventMsg" structure
// and returns that structure.
func EventMsgParser(strFormat string) ranptpparameters.EventMsg {
	var eventMassage ranptpparameters.EventMsg

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
func eventDataParser(eventDataStr string) ranptpparameters.EventData {
	var eventDataFull ranptpparameters.EventData

	versionStart := eventDataStr[strings.Index(eventDataStr, "version")+13:]
	valuesArray := versionStart[strings.Index(versionStart, "[") : strings.Index(versionStart, "]")+1]

	eventDataFull.Version = extractDetail(versionStart)
	eventDataFull.Values = eventDataValuesParser(valuesArray)

	return eventDataFull
}

// eventDataValuesParser gets the "Values" from the PTP event data as a string and parsers it into a "values" structure
// and returns an array of "values" structure.
func eventDataValuesParser(eventDataValuesStr string) []ranptpparameters.Values {
	var eventDataValuesFull []ranptpparameters.Values

	str := eventDataValuesStr

	for strings.Contains(str, "resource") {
		resourcesStart := str[strings.Index(str, "resource")+15:]
		dataTypeStart := resourcesStart[strings.Index(resourcesStart, "dataType")+14:]
		valueTypeStart := dataTypeStart[strings.Index(dataTypeStart, "valueType")+15:]
		valueStart := valueTypeStart[strings.Index(valueTypeStart, "value")+11:]

		eventDataValuesFull = append(eventDataValuesFull,
			ranptpparameters.Values{Resource: extractDetail(resourcesStart), DataType: extractDetail(dataTypeStart),
				ValueType: extractDetail(valueTypeStart), Value: extractDetail(valueStart)})
		str = valueStart
	}

	return eventDataValuesFull
}

// extractDetail gets a string with the "\"" characters
// and returns a substring from the begging of the given string until the first ""\"" characters.
func extractDetail(start string) string {
	return start[:strings.Index(start, "\"")-1]
}
