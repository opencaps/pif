package driver

import (
	"reflect"
	"strconv"
	"strings"
)

// Translation translation formula
type Translation struct {
	Field string
	Map   map[interface{}]interface{}
	A     float64
}

func (t *Translation) init(formula *Formula, formulaExit bool, forceToString bool) {
	if !formulaExit {
		t.A = 1
		t.Map = nil
		return
	}

	if formula.A != nil {
		t.A = *formula.A
		log.Info("Translation forumla A:", t.A)
	} else {
		log.Info("Translation forumla A nil")
		t.A = 1
	}

	t.Map = make(map[interface{}]interface{})
	if formula.Map == "" {
		return
	}

	for _, tupleRaw := range strings.Split(formula.Map, ";") {
		tuple := strings.ReplaceAll(tupleRaw, "(", "")
		tuple = strings.ReplaceAll(tuple, ")", "")
		keyValue := strings.Split(tuple, ",")
		if len(keyValue) != 2 {
			log.Info("translation formula tuple not valid:", tupleRaw)
			continue
		}

		key := convert(keyValue[0])

		if forceToString {
			t.Map[key] = keyValue[1]
		} else {
			t.Map[key] = convert(keyValue[1])
		}
	}
}

func convert(data string) interface{} {
	f, err := strconv.ParseFloat(data, 64)
	if err == nil {
		return f
	}

	b, err := strconv.ParseBool(data)
	if err == nil {
		return b
	}

	return data
}

// Translate to convert a data into the right format
func (t *Translation) Translate(data interface{}) interface{} {
	value := t.translateMap(data)

	if t.A != 1 {
		value = t.translateCoeff(value)
	}

	return value
}

func (t *Translation) translateCoeff(value interface{}) interface{} {
	f64, ok := value.(float64)
	if ok {
		if t.A > 1 {
			return int64(f64 * t.A)
		}
		return f64 * t.A
	}

	f32, ok := value.(float32)
	if ok {
		if t.A > 1 {
			return int64(float64(f32) * t.A)
		}
		return float64(f32) * t.A
	}

	i64, ok := value.(int64)
	if ok {
		if t.A > 1 {
			return int64(float64(i64) * t.A)
		}
		return float64(i64) * t.A
	}

	i, ok := value.(int)
	if ok {
		if t.A > 1 {
			return int64(float64(i) * t.A)
		}
		return float64(i) * t.A
	}

	log.Warning("Value", value, reflect.TypeOf(value), "not able to use coeff A", t.A)
	return value
}

func (t *Translation) translateMap(data interface{}) interface{} {
	if len(t.Map) > 0 {
		for key, value := range t.Map {
			if key == data {
				return value
			} else if reflect.TypeOf(data).Kind() == reflect.Int &&
				reflect.TypeOf(key).Kind() == reflect.Float64 && key == float64(data.(int)) {
				// data is a int equals to the key which is a float64
				return value
			}
		}

		log.Warning("No translation found for data:", data, "(", reflect.TypeOf(data), ") with the map:", t.Map)
	}

	return data
}
