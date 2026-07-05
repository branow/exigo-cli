package exigoapi

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"exigo-cli/internal/credentials"
)

// buildEnvelope constructs a SOAP 1.1 envelope invoking operation with
// fields as its request body's child elements, carrying creds in the
// ApiAuthentication header. Only LoginName/Password/Company are sent —
// research surfaced additional speculative header fields (Identity,
// RequestTimeUtc, Signature) that could not be confirmed against the
// live WSDL, so they're omitted rather than guessed.
func buildEnvelope(creds credentials.Credentials, operation string, fields map[string]string) []byte {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">`)

	b.WriteString("<soap:Header>")
	fmt.Fprintf(&b, `<ApiAuthentication xmlns=%q>`, targetNamespace)
	writeElement(&b, "LoginName", creds.LoginName)
	writeElement(&b, "Password", creds.Password)
	writeElement(&b, "Company", creds.Company)
	b.WriteString("</ApiAuthentication>")
	b.WriteString("</soap:Header>")

	b.WriteString("<soap:Body>")
	fmt.Fprintf(&b, `<%s xmlns=%q>`, operation, targetNamespace)
	for _, key := range sortedKeys(fields) {
		writeElement(&b, key, fields[key])
	}
	fmt.Fprintf(&b, `</%s>`, operation)
	b.WriteString("</soap:Body>")

	b.WriteString("</soap:Envelope>")
	return []byte(b.String())
}

func writeElement(b *strings.Builder, name, value string) {
	fmt.Fprintf(b, "<%s>%s</%s>", name, xmlEscape(value), name)
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// sortedKeys returns fields' keys in a deterministic order so the built
// envelope is stable and testable.
func sortedKeys(fields map[string]string) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
