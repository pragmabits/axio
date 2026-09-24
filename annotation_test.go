package axio

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestField(t *testing.T) {
	note := Field("key", "value")
	assertEqual(t, note.Name(), "key")
	assertEqual(t, note.Data().(string), "value")
}

func TestAnnotation_Name_Data(t *testing.T) {
	annotation := Field("k", "v1")
	assertEqual(t, annotation.Name(), "k")
	assertEqual(t, annotation.Data().(string), "v1")
}

func TestAnnotation_Data(t *testing.T) {
	moment := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	t.Run("duration", func(t *testing.T) {
		assertEqual(t, Field("timeout", time.Second).Data(), any(time.Second))
	})
	t.Run("time", func(t *testing.T) {
		value, ok := Field("at", moment).Data().(time.Time)
		assertEqual(t, ok, true)
		assertEqual(t, value.Equal(moment), true)
	})
	t.Run("uintptr", func(t *testing.T) {
		assertEqual(t, Field("address", uintptr(7)).Data(), any(uint64(7)))
	})

	slices := []struct {
		name  string
		value any
	}{
		{"ints", []int{1, 2}},
		{"strings", []string{"a", "b"}},
		{"bools", []bool{true}},
		{"durations", []time.Duration{time.Second}},
		{"times", []time.Time{moment}},
		{"errors", []error{errors.New("declined")}},
	}
	for _, slice := range slices {
		t.Run(slice.name+"_as_given", func(t *testing.T) {
			data := Field("values", slice.value).Data()
			assertEqual(t, reflect.TypeOf(data), reflect.TypeOf(slice.value))
			assertEqual(t, fmt.Sprint(data), fmt.Sprint(slice.value))
		})
	}
	t.Run("array_marshaler_as_given", func(t *testing.T) {
		assertEqual(t, Field("items", piiFailingArray{item: "x"}).Data(), any(piiFailingArray{item: "x"}))
	})
}

func TestAnnotation_Value(t *testing.T) {
	moment := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	t.Run("int_as_int", func(t *testing.T) {
		value, ok := Field("count", 3).Value[int]()
		assertEqual(t, ok, true)
		assertEqual(t, value, 3)
	})
	t.Run("int_as_int64", func(t *testing.T) {
		value, ok := Field("count", 3).Value[int64]()
		assertEqual(t, ok, true)
		assertEqual(t, value, int64(3))
	})
	t.Run("int_as_int8_when_it_fits", func(t *testing.T) {
		value, ok := Field("count", 100).Value[int8]()
		assertEqual(t, ok, true)
		assertEqual(t, value, int8(100))
	})
	t.Run("int_as_int8_when_it_overflows", func(t *testing.T) {
		_, ok := Field("count", 1000).Value[int8]()
		assertEqual(t, ok, false)
	})
	t.Run("negative_int_as_uint", func(t *testing.T) {
		_, ok := Field("count", -1).Value[uint]()
		assertEqual(t, ok, false)
	})
	t.Run("uint_as_uint", func(t *testing.T) {
		value, ok := Field("count", uint(7)).Value[uint]()
		assertEqual(t, ok, true)
		assertEqual(t, value, uint(7))
	})
	t.Run("float32_as_float32", func(t *testing.T) {
		value, ok := Field("ratio", float32(0.5)).Value[float32]()
		assertEqual(t, ok, true)
		assertEqual(t, value, float32(0.5))
	})
	t.Run("string_as_string", func(t *testing.T) {
		value, ok := Field("user_id", "usr_1").Value[string]()
		assertEqual(t, ok, true)
		assertEqual(t, value, "usr_1")
	})
	t.Run("string_as_int", func(t *testing.T) {
		value, ok := Field("user_id", "usr_1").Value[int]()
		assertEqual(t, ok, false)
		assertEqual(t, value, 0)
	})
	t.Run("struct_as_itself", func(t *testing.T) {
		value, ok := Field("http", HTTP{Method: "GET"}).Value[HTTP]()
		assertEqual(t, ok, true)
		assertEqual(t, value.Method, "GET")
	})
	t.Run("duration_as_duration", func(t *testing.T) {
		value, ok := Field("timeout", time.Second).Value[time.Duration]()
		assertEqual(t, ok, true)
		assertEqual(t, value, time.Second)
	})
	t.Run("time_as_time", func(t *testing.T) {
		value, ok := Field("at", moment).Value[time.Time]()
		assertEqual(t, ok, true)
		assertEqual(t, value.Equal(moment), true)
	})
	t.Run("int_slice_as_int_slice", func(t *testing.T) {
		value, ok := Field("ids", []int{1, 2}).Value[[]int]()
		assertEqual(t, ok, true)
		assertEqual(t, fmt.Sprint(value), "[1 2]")
	})
}

func TestAnnotations_Names_Data_Add(t *testing.T) {
	var annotations Annotations
	annotations = append(annotations, Field("a", 1))
	annotations = append(annotations, Field("b", 2))

	names := annotations.Names()
	assertEqual(t, len(names), 2)
	assertEqual(t, names[0], "a")
	assertEqual(t, names[1], "b")

	data := annotations.Data()
	assertEqual(t, len(data), 2)
	assertEqual(t, data[0].(int64), int64(1))
	assertEqual(t, data[1].(int64), int64(2))

	annotations.Add("c", 3)
	assertEqual(t, len(annotations), 3)
	assertEqual(t, annotations[2].Name(), "c")
}

func TestAnnotations_Add(t *testing.T) {
	t.Run("primitive_value_does_not_allocate", func(t *testing.T) {
		annotations := make(Annotations, 0, 1)
		count, ratio, index := 15999, 0.75, 0
		users := []string{"usr_456", "usr_789"}
		tests := []struct {
			name string
			add  func()
		}{
			{
				name: "int",
				add: func() {
					count++
					annotations.Add("count", count)
				},
			},
			{
				name: "float64",
				add: func() {
					ratio += 0.25
					annotations.Add("ratio", ratio)
				},
			},
			{
				name: "string",
				add: func() {
					index++
					annotations.Add("user_id", users[index%len(users)])
				},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				allocations := testing.AllocsPerRun(100, func() {
					annotations = annotations[:0]
					test.add()
				})
				if allocations != 0 {
					t.Errorf("Add allocated %v times per call, want 0", allocations)
				}
			})
		}
	})
}

func TestHTTP_Annotable(t *testing.T) {
	request := HTTP{Method: "GET", URL: "/api", StatusCode: 200, LatencyMS: 5, UserAgent: "test", ClientIP: "127.0.0.1"}
	annotations := request.Append(nil)

	assertEqual(t, len(annotations), 6)
	assertEqual(t, annotations[0].Name(), "method")
	assertEqual(t, annotations[0].Data().(string), "GET")
	assertEqual(t, annotations[1].Name(), "url")
	assertEqual(t, annotations[1].Data().(string), "/api")
	assertEqual(t, annotations[2].Name(), "status_code")
	assertEqual(t, annotations[2].Data().(int64), int64(200))
}

func TestField_Types(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		annotation := Field("key", "hello")
		assertEqual(t, annotation.Data().(string), "hello")
	})

	t.Run("int", func(t *testing.T) {
		annotation := Field("key", 42)
		assertEqual(t, annotation.Data().(int64), int64(42))
	})

	t.Run("bool", func(t *testing.T) {
		annotation := Field("key", true)
		assertEqual(t, annotation.Data().(bool), true)
	})

	t.Run("float64", func(t *testing.T) {
		annotation := Field("key", 3.14)
		data := annotation.Data().(float64)
		if data < 3.13 || data > 3.15 {
			t.Errorf("expected ~3.14, got %f", data)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type order struct{ ID string }
		annotation := Field("key", order{ID: "123"})
		data := annotation.Data().(order)
		assertEqual(t, data.ID, "123")
	})
}
