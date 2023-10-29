package dataformat

import (
	"testing"
)

func TestDataformatDecodeEncode_JSON(t *testing.T) {
	obj := `{"foo":"bar"}`

	decoded, err := Decode(obj)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DataFormat != "json" {
		t.Fatal("unexpected data format")
	}
	if decoded.Data["foo"] != "bar" {
		t.Fatal("unexpected data")
	}

	encoded, err := Encode(decoded.Data, decoded.DataFormat)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != obj {
		t.Fatal("unexpected data")
	}
}

func TestDataformatDecodeEncode_XML(t *testing.T) {
	obj := `<foo attr="baz">bar</foo>`

	decoded, err := Decode(obj)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DataFormat != "xml" {
		t.Fatal("unexpected data format")
	}
	if decoded.Data["foo"].(map[string]interface{})["#text"] != "bar" {
		t.Fatal("unexpected data")
	}
	if decoded.Data["foo"].(map[string]interface{})["-attr"] != "baz" {
		t.Fatal("unexpected data")
	}

	encoded, err := Encode(decoded.Data, decoded.DataFormat)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != obj {
		t.Fatal("unexpected data")
	}
}

func TestDataformatDecodeEncode_Form(t *testing.T) {
	obj := "foo=bar"

	decoded, err := Decode(obj)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DataFormat != "form" {
		t.Fatal("unexpected data format")
	}
	if decoded.Data["foo"] != "bar" {
		t.Fatal("unexpected data")
	}

	encoded, err := Encode(decoded.Data, decoded.DataFormat)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != obj {
		t.Fatal("unexpected data")
	}
}

/*
	func TestDataformatDecodeEncode_Multipart(t *testing.T) {
		obj := "--boundary\r\nContent-Disposition: form-data; name=\"foo\"\r\n\r\nbar\r\n--boundary--"

		decoded, err := Decode(obj)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.DataFormat != "multipart" {
			t.Fatal("unexpected data format")
		}
		if decoded.Data["foo"] != "bar" {
			t.Fatal("unexpected data")
		}

		encoded, err := Encode(decoded.Data, decoded.DataFormat)
		if err != nil {
			t.Fatal(err)
		}
		if encoded != obj {
			t.Fatal("unexpected data")
		}
	}
*/

func TestDataformatDecodeEncode_Raw(t *testing.T) {
	obj := "foo"

	decoded, err := Decode(obj)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DataFormat != "raw" {
		t.Fatal("unexpected data format")
	}
	if decoded.Data["value"] != "foo" {
		t.Fatal("unexpected data")
	}

	encoded, err := Encode(decoded.Data, decoded.DataFormat)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != obj {
		t.Fatal("unexpected data")
	}
}
