package axio

import (
	"testing"
)

func TestAnnotate(t *testing.T) {
	note := Annotate("key", "value")
	assertEqual(t, note.Name(), "key")
	assertEqual(t, note.Data().(string), "value")
}

func TestAnnotation_Name_Data_Set(t *testing.T) {
	annotation := Annotate("k", "v1")
	assertEqual(t, annotation.Name(), "k")
	assertEqual(t, annotation.Data().(string), "v1")

	annotation.Set("v2")
	assertEqual(t, annotation.Data().(string), "v2")
}

func TestAnnotations_Names_Data_Add(t *testing.T) {
	var annotations Annotations
	annotations = append(annotations, Annotate("a", 1))
	annotations = append(annotations, Annotate("b", 2))

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

func TestHTTP_Annotable(t *testing.T) {
	h := HTTP{Method: "GET", URL: "/api", StatusCode: 200, LatencyMS: 5, UserAgent: "test", ClientIP: "127.0.0.1"}
	annotations := h.Append(nil)

	assertEqual(t, len(annotations), 6)
	assertEqual(t, annotations[0].Name(), "method")
	assertEqual(t, annotations[0].Data().(string), "GET")
	assertEqual(t, annotations[1].Name(), "url")
	assertEqual(t, annotations[1].Data().(string), "/api")
	assertEqual(t, annotations[2].Name(), "status_code")
	assertEqual(t, annotations[2].Data().(int64), int64(200))
}

func TestAnnotate_Types(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		annotation := Annotate("key", "hello")
		assertEqual(t, annotation.Data().(string), "hello")
	})

	t.Run("int", func(t *testing.T) {
		annotation := Annotate("key", 42)
		assertEqual(t, annotation.Data().(int64), int64(42))
	})

	t.Run("bool", func(t *testing.T) {
		annotation := Annotate("key", true)
		assertEqual(t, annotation.Data().(bool), true)
	})

	t.Run("float64", func(t *testing.T) {
		annotation := Annotate("key", 3.14)
		data := annotation.Data().(float64)
		if data < 3.13 || data > 3.15 {
			t.Errorf("expected ~3.14, got %f", data)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type order struct{ ID string }
		annotation := Annotate("key", order{ID: "123"})
		data := annotation.Data().(order)
		assertEqual(t, data.ID, "123")
	})
}
