package exigoapi

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Result is the decoded {Operation}Result element of a completed SOAP
// response: its child elements as a generic, JSON-friendly tree, plus any
// business-level errors the operation reported via its Errors[] array.
type Result struct {
	Fields map[string]any
	Errors []string
}

type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	Fault   *soapFault `xml:"Fault"`
	Content []byte     `xml:",innerxml"`
}

type soapFault struct {
	Code   string `xml:"faultcode"`
	String string `xml:"faultstring"`
}

// parseResponse unwraps a SOAP envelope, returning either a decoded
// envelope-level *Fault error or the named operation's *Result, itself
// carrying a *BusinessError when the result's Errors[] is non-empty.
func parseResponse(operation string, body []byte) (*Result, error) {
	var envelope soapEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decoding SOAP envelope: %w", err)
	}
	if envelope.Body.Fault != nil {
		return nil, &Fault{Code: envelope.Body.Fault.Code, String: envelope.Body.Fault.String}
	}

	result, err := decodeResult(operation, envelope.Body.Content)
	if err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		return result, &BusinessError{Operation: operation, Errors: result.Errors}
	}
	return result, nil
}

// decodeResult scans content for the {operation}Result element and
// decodes its children generically.
func decodeResult(operation string, content []byte) (*Result, error) {
	resultName := operation + "Result"
	decoder := xml.NewDecoder(bytes.NewReader(content))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("response did not contain a %s element", resultName)
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != resultName {
			continue
		}
		node, err := decodeNode(decoder)
		if err != nil {
			return nil, err
		}
		fields, _ := node.(map[string]any)
		if fields == nil {
			fields = map[string]any{}
		}
		return &Result{Fields: fields, Errors: extractErrors(fields)}, nil
	}
}

// decodeNode reads tokens up to and including the matching EndElement,
// returning trimmed text content for a leaf element or a map of children
// (repeated child names collapsing into a slice) for a branch element.
func decodeNode(d *xml.Decoder) (any, error) {
	children := map[string]any{}
	hasChildren := false
	var text strings.Builder

	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			hasChildren = true
			value, err := decodeNode(d)
			if err != nil {
				return nil, err
			}
			appendChild(children, t.Name.Local, value)
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if hasChildren {
				return children, nil
			}
			return strings.TrimSpace(text.String()), nil
		}
	}
}

func appendChild(children map[string]any, name string, value any) {
	existing, ok := children[name]
	if !ok {
		children[name] = value
		return
	}
	if list, ok := existing.([]any); ok {
		children[name] = append(list, value)
		return
	}
	children[name] = []any{existing, value}
}

// extractErrors normalizes a decoded Errors element (the ArrayOfString
// convention .NET/WCF serializes as repeated <string> children) into a
// plain string slice, handling the empty, single-item, and multi-item
// shapes decodeNode can produce.
func extractErrors(fields map[string]any) []string {
	raw, ok := fields["Errors"]
	if !ok {
		return nil
	}
	if s, ok := raw.(string); ok {
		return nonEmptyStrings([]any{s})
	}
	if m, ok := raw.(map[string]any); ok {
		return errorStrings(m["string"])
	}
	return nil
}

func errorStrings(v any) []string {
	if list, ok := v.([]any); ok {
		return nonEmptyStrings(list)
	}
	return nonEmptyStrings([]any{v})
}

func nonEmptyStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
