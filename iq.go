package xmpp

import (
	"encoding/xml"
	"fmt"
)

/*
TODO support ability to put Raw payload inside IQ
*/

// ============================================================================
// IQ Packet

// IQ implements RFC 6120 - A.5 Client Namespace (a part)
type IQ struct { // Info/Query
	XMLName xml.Name `xml:"iq"`
	// MUST have a ID
	Attrs
	// We can only have one payload on IQ:
	//   "An IQ stanza of type "get" or "set" MUST contain exactly one
	//    child element, which specifies the semantics of the particular
	//    request."
	Payload IQPayload `xml:",omitempty"`
	Error   Err       `xml:"error,omitempty"`
	RawXML  string    `xml:",innerxml"`
}

type IQPayload interface {
	Namespace() string
}

func NewIQ(a Attrs) IQ {
	// TODO generate IQ ID if not set
	// TODO ensure that type is set, as it is required
	return IQ{
		XMLName: xml.Name{Local: "iq"},
		Attrs:   a,
	}
}

func (iq IQ) MakeError(xerror Err) IQ {
	from := iq.From
	to := iq.To

	iq.Type = "error"
	iq.From = to
	iq.To = from
	iq.Error = xerror

	return iq
}

func (IQ) Name() string {
	return "iq"
}

type iqDecoder struct{}

var iq iqDecoder

func (iqDecoder) decode(p *xml.Decoder, se xml.StartElement) (IQ, error) {
	var packet IQ
	err := p.DecodeElement(&packet, &se)
	return packet, err
}

// UnmarshalXML implements custom parsing for IQs
func (iq *IQ) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	iq.XMLName = start.Name

	// Extract IQ attributes
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			iq.Id = attr.Value
		}
		if attr.Name.Local == "type" {
			iq.Type = StanzaType(attr.Value)
		}
		if attr.Name.Local == "to" {
			iq.To = attr.Value
		}
		if attr.Name.Local == "from" {
			iq.From = attr.Value
		}
	}

	// decode inner elements
	for {
		t, err := d.Token()
		if err != nil {
			return err
		}

		switch tt := t.(type) {
		case xml.StartElement:
			if tt.Name.Local == "error" {
				var xmppError Err
				err = d.DecodeElement(&xmppError, &tt)
				if err != nil {
					fmt.Println(err)
					return err
				}
				iq.Error = xmppError
				continue
			}
			if iqExt := TypeRegistry.GetIQExtension(tt.Name); iqExt != nil {
				// Decode payload extension
				err = d.DecodeElement(iqExt, &tt)
				if err != nil {
					return err
				}
				iq.Payload = iqExt
				continue
			}
			// TODO: If unknown decode as generic node
			return fmt.Errorf("unexpected element in iq: %s %s", tt.Name.Space, tt.Name.Local)
		case xml.EndElement:
			if tt == start.End() {
				return nil
			}
		}
	}
}

// ============================================================================
// Generic / unknown content

// Node is a generic structure to represent XML data. It is used to parse
// unreferenced or custom stanza payload.
type Node struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:"-"`
	Content string     `xml:",innerxml"`
	Nodes   []Node     `xml:",any"`
}

func (n *Node) Namespace() string {
	return n.XMLName.Space
}

// Attr represents generic XML attributes, as used on the generic XML Node
// representation.
type Attr struct {
	K string
	V string
}

// UnmarshalXML is a custom unmarshal function used by xml.Unmarshal to
// transform generic XML content into hierarchical Node structure.
func (n *Node) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// Assign	"n.Attrs = start.Attr", without repeating xmlns in attributes:
	for _, attr := range start.Attr {
		// Do not repeat xmlns, it is already in XMLName
		if attr.Name.Local != "xmlns" {
			n.Attrs = append(n.Attrs, attr)
		}
	}
	type node Node
	return d.DecodeElement((*node)(n), &start)
}

// MarshalXML is a custom XML serializer used by xml.Marshal to serialize a
// Node structure to XML.
func (n Node) MarshalXML(e *xml.Encoder, start xml.StartElement) (err error) {
	start.Attr = n.Attrs
	start.Name = n.XMLName

	err = e.EncodeToken(start)
	e.EncodeElement(n.Nodes, xml.StartElement{Name: n.XMLName})
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// ============================================================================
// Disco

const (
	NSDiscoInfo  = "http://jabber.org/protocol/disco#info"
	NSDiscoItems = "http://jabber.org/protocol/disco#items"
)

// Disco Info
type DiscoInfo struct {
	XMLName  xml.Name  `xml:"http://jabber.org/protocol/disco#info query"`
	Node     string    `xml:"node,attr,omitempty"`
	Identity Identity  `xml:"identity"`
	Features []Feature `xml:"feature"`
}

func (d *DiscoInfo) Namespace() string {
	return d.XMLName.Space
}

type Identity struct {
	XMLName  xml.Name `xml:"identity,omitempty"`
	Name     string   `xml:"name,attr,omitempty"`
	Category string   `xml:"category,attr,omitempty"`
	Type     string   `xml:"type,attr,omitempty"`
}

type Feature struct {
	XMLName xml.Name `xml:"feature"`
	Var     string   `xml:"var,attr"`
}

// Disco Items
type DiscoItems struct {
	XMLName xml.Name    `xml:"http://jabber.org/protocol/disco#items query"`
	Node    string      `xml:"node,attr,omitempty"`
	Items   []DiscoItem `xml:"item"`
}

func (d *DiscoItems) Namespace() string {
	return d.XMLName.Space
}

type DiscoItem struct {
	XMLName xml.Name `xml:"item"`
	Name    string   `xml:"name,attr,omitempty"`
	JID     string   `xml:"jid,attr,omitempty"`
	Node    string   `xml:"node,attr,omitempty"`
}

// ============================================================================
// Software Version (XEP-0092)

// Version
type Version struct {
	XMLName xml.Name `xml:"jabber:iq:version query"`
	Name    string   `xml:"name,omitempty"`
	Version string   `xml:"version,omitempty"`
	OS      string   `xml:"os,omitempty"`
}

func (v *Version) Namespace() string {
	return v.XMLName.Space
}

// ============================================================================
// Registry init

func init() {
	TypeRegistry.MapExtension(PKTIQ, xml.Name{NSDiscoInfo, "query"}, DiscoInfo{})
	TypeRegistry.MapExtension(PKTIQ, xml.Name{NSDiscoItems, "query"}, DiscoItems{})
	TypeRegistry.MapExtension(PKTIQ, xml.Name{"urn:ietf:params:xml:ns:xmpp-bind", "bind"}, BindBind{})
	TypeRegistry.MapExtension(PKTIQ, xml.Name{"urn:xmpp:iot:control", "set"}, ControlSet{})
	TypeRegistry.MapExtension(PKTIQ, xml.Name{"jabber:iq:version", "query"}, Version{})
}
