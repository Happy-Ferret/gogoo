package utility_test

import (
	"log"
	"testing"

	. "github.com/iKala/gogoo/utility"
)

func TestJSONStringToMap(t *testing.T) {
	testedJSONString := "{\"hash\":\"ww9xPA7Abvwf8CTcih\",\"name\":\"joe\"}"

	result := JSONStringToMap(testedJSONString)
	log.Printf("result: %+v", result)
}

func TestJSONMapToString(t *testing.T) {

	testedJSONMap := map[string]interface{}{
		"name": "leo",
		"age":  10,
	}

	result := JSONMapToString(testedJSONMap)

	log.Printf("result: %+v", result)
}

func TestJSONStringToList(t *testing.T) {
	testedString := "[[\"10.240.0.80\",\"10.240.0.12\"],[\"10.240.0.113\"]]"
	result := [][]string{}

	array := JSONStringToList(testedString)
	for _, subArr := range array {
		group := []string{}
		for _, ele := range subArr.([]interface{}) {
			group = append(group, ele.(string))
		}
		result = append(result, group)
	}
	log.Printf("RtmpGroup: %+v", result)
}

func TestPrettyPrintJSONMap(t *testing.T) {
	testedJSONMap := map[string]interface{}{"apple": 5, "lettuce": 7}

	PrettyPrintJSONMap(testedJSONMap)
}
