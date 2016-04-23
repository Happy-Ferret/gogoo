package utility

import (
	"encoding/json"
	log "github.com/cihub/seelog"
)

// JSONStringToMap converts json string to map
func JSONStringToMap(jsonString string) map[string]interface{} {
	var dat map[string]interface{}

	err := json.Unmarshal([]byte(jsonString), &dat)
	if err != nil {
		log.Debugf("Unmarshal error: jsonString[%s], error[%s]", jsonString, err.Error())
	}

	return dat
}

// JSONMapToString converts json map to string
func JSONMapToString(jsonMap map[string]interface{}) []byte {
	dat, err := json.Marshal(jsonMap)
	if err != nil {
		log.Warn(err.Error())
	}

	return dat
}

// PrettyPrintJSONMap print json map with indent
func PrettyPrintJSONMap(jsonMap map[string]interface{}) {
	result, _ := json.MarshalIndent(jsonMap, "", "   ")

	log.Trace(string(result))
}

// JSONStringToList converts json string to list
func JSONStringToList(jsonString string) []interface{} {
	var dat []interface{}

	err := json.Unmarshal([]byte(jsonString), &dat)
	if err != nil {
		log.Debugf("Unmarshal error: jsonString[%s], error[%s]", jsonString, err.Error())
	}

	return dat
}
