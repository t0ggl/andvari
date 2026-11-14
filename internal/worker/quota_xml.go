package worker

import (
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// UnlimitedQuotaCeiling defines the upper ceiling in bytes used when reporting available quota
// for users with unlimited storage (RFC 4331 § 3).
const UnlimitedQuotaCeiling int64 = math.MaxInt64

// CalculateAvailableBytes calculates the quota-available-bytes according to RFC 4331 § 3.
// If quotaLimit is > 0, returns quotaLimit - usedBytes (or 0 if exceeded).
// If quota is unlimited (quotaLimit <= 0), returns the ceiling minus usedBytes.
func CalculateAvailableBytes(quotaLimit, usedBytes int64) int64 {
	if quotaLimit > 0 {
		available := quotaLimit - usedBytes
		if available < 0 {
			return 0
		}
		return available
	}
	if usedBytes > 0 && UnlimitedQuotaCeiling > usedBytes {
		return UnlimitedQuotaCeiling - usedBytes
	}
	return UnlimitedQuotaCeiling
}

// ParsePropfindRequest inspects the PROPFIND request XML body to determine if the client
// requested quota-available-bytes, quota-used-bytes, or allprop.
// An empty body is treated as allprop per RFC 4918 § 9.1.
func ParsePropfindRequest(body []byte) (wantAvailable bool, wantUsed bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return true, true
	}

	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			local := se.Name.Local
			if idx := strings.Index(local, ":"); idx != -1 {
				local = local[idx+1:]
			}
			switch local {
			case "allprop":
				return true, true
			case "quota-available-bytes":
				wantAvailable = true
			case "quota-used-bytes":
				wantUsed = true
			}
		}
	}
	return wantAvailable, wantUsed
}

// ResponseBuffer captures an HTTP response for post-processing before writing to the client.
type ResponseBuffer struct {
	StatusCode int
	HeaderMap  http.Header
	Body       bytes.Buffer
}

func NewResponseBuffer() *ResponseBuffer {
	return &ResponseBuffer{
		HeaderMap: make(http.Header),
	}
}

func (r *ResponseBuffer) Header() http.Header {
	return r.HeaderMap
}

func (r *ResponseBuffer) WriteHeader(statusCode int) {
	r.StatusCode = statusCode
}

func (r *ResponseBuffer) Write(b []byte) (int, error) {
	if r.StatusCode == 0 {
		r.StatusCode = http.StatusOK
	}
	return r.Body.Write(b)
}

type XMLNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr
	Children []*XMLNode
	Text     string
}

func parseXMLTree(r io.Reader) (*XMLNode, error) {
	dec := xml.NewDecoder(r)
	var root *XMLNode
	var stack []*XMLNode

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			n := &XMLNode{
				XMLName: t.Name,
				Attrs:   t.Attr,
			}
			if len(stack) == 0 {
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}
	return root, nil
}

func localName(n *XMLNode) string {
	name := n.XMLName.Local
	if idx := strings.Index(name, ":"); idx != -1 {
		return name[idx+1:]
	}
	return name
}

func findChild(n *XMLNode, local string) *XMLNode {
	for _, c := range n.Children {
		if localName(c) == local {
			return c
		}
	}
	return nil
}

func davPrefixTag(tag, prefix string) string {
	if prefix != "" {
		return prefix + ":" + tag
	}
	return tag
}

func (n *XMLNode) writeTo(buf *bytes.Buffer) {
	name := n.XMLName.Local
	if n.XMLName.Space == "DAV:" || strings.HasPrefix(n.XMLName.Local, "D:") {
		if !strings.HasPrefix(name, "D:") {
			name = "D:" + name
		}
	}
	buf.WriteString("<" + name)
	for _, a := range n.Attrs {
		var attrName string
		if a.Name.Space == "xmlns" {
			attrName = "xmlns:" + a.Name.Local
		} else if a.Name.Space != "" {
			attrName = a.Name.Space + ":" + a.Name.Local
		} else {
			attrName = a.Name.Local
		}
		buf.WriteString(" " + attrName + "=\"" + a.Value + "\"")
	}
	if len(n.Children) == 0 && strings.TrimSpace(n.Text) == "" {
		buf.WriteString("/>")
		return
	}
	buf.WriteString(">")
	if len(n.Children) == 0 {
		xml.EscapeText(buf, []byte(strings.TrimSpace(n.Text)))
	} else {
		for _, child := range n.Children {
			child.writeTo(buf)
		}
	}
	buf.WriteString("</" + name + ">")
}

// InjectQuotaProperties modifies a WebDAV 207 Multi-Status XML response to dynamically inject
// RFC 4331 quota properties (quota-available-bytes and quota-used-bytes).
func InjectQuotaProperties(respXML []byte, quotaLimit, usedBytes int64, wantAvailable, wantUsed bool) ([]byte, error) {
	if !wantAvailable && !wantUsed {
		return respXML, nil
	}

	root, err := parseXMLTree(bytes.NewReader(respXML))
	if err != nil {
		return nil, err
	}

	if localName(root) != "multistatus" {
		return respXML, nil
	}

	// Detect DAV prefix from root attributes (default "D")
	prefix := "D"
	for _, a := range root.Attrs {
		if a.Value == "DAV:" {
			if a.Name.Space == "xmlns" {
				prefix = a.Name.Local
			} else if a.Name.Local == "xmlns" {
				prefix = ""
			}
		}
	}

	availableBytes := CalculateAvailableBytes(quotaLimit, usedBytes)
	availStr := strconv.FormatInt(availableBytes, 10)
	usedStr := strconv.FormatInt(usedBytes, 10)

	for _, resp := range root.Children {
		if localName(resp) != "response" {
			continue
		}

		var propstat200 *XMLNode
		for _, c := range resp.Children {
			if localName(c) == "propstat" {
				statusNode := findChild(c, "status")
				if statusNode != nil && strings.Contains(statusNode.Text, "200") {
					propstat200 = c
					break
				}
			}
		}

		// Filter out requested quota properties from any 404 propstat
		var filteredChildren []*XMLNode
		for _, c := range resp.Children {
			if localName(c) == "propstat" {
				statusNode := findChild(c, "status")
				if statusNode != nil && strings.Contains(statusNode.Text, "404") {
					prop404 := findChild(c, "prop")
					if prop404 != nil {
						var remaining []*XMLNode
						for _, p := range prop404.Children {
							name := localName(p)
							if (name == "quota-available-bytes" && wantAvailable) || (name == "quota-used-bytes" && wantUsed) {
								continue
							}
							remaining = append(remaining, p)
						}
						prop404.Children = remaining
						if len(prop404.Children) == 0 {
							// Omit empty 404 propstat
							continue
						}
					}
				}
			}
			filteredChildren = append(filteredChildren, c)
		}
		resp.Children = filteredChildren

		// Ensure 200 OK propstat exists
		if propstat200 == nil {
			propNode := &XMLNode{
				XMLName: xml.Name{Space: "DAV:", Local: davPrefixTag("prop", prefix)},
			}
			statusNode := &XMLNode{
				XMLName: xml.Name{Space: "DAV:", Local: davPrefixTag("status", prefix)},
				Text:    "HTTP/1.1 200 OK",
			}
			propstat200 = &XMLNode{
				XMLName:  xml.Name{Space: "DAV:", Local: davPrefixTag("propstat", prefix)},
				Children: []*XMLNode{propNode, statusNode},
			}
			resp.Children = append(resp.Children, propstat200)
		}

		prop200 := findChild(propstat200, "prop")
		if prop200 == nil {
			prop200 = &XMLNode{
				XMLName: xml.Name{Space: "DAV:", Local: davPrefixTag("prop", prefix)},
			}
			propstat200.Children = append([]*XMLNode{prop200}, propstat200.Children...)
		}

		if wantAvailable {
			availNode := findChild(prop200, "quota-available-bytes")
			if availNode != nil {
				availNode.Text = availStr
				availNode.Children = nil
			} else {
				prop200.Children = append(prop200.Children, &XMLNode{
					XMLName: xml.Name{Space: "DAV:", Local: davPrefixTag("quota-available-bytes", prefix)},
					Text:    availStr,
				})
			}
		}

		if wantUsed {
			usedNode := findChild(prop200, "quota-used-bytes")
			if usedNode != nil {
				usedNode.Text = usedStr
				usedNode.Children = nil
			} else {
				prop200.Children = append(prop200.Children, &XMLNode{
					XMLName: xml.Name{Space: "DAV:", Local: davPrefixTag("quota-used-bytes", prefix)},
					Text:    usedStr,
				})
			}
		}
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	root.writeTo(&buf)
	return buf.Bytes(), nil
}
