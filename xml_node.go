package ttml

import (
	"encoding/xml"
	"io"
	"strings"
)

type nodeType int

const (
	nodeDocument nodeType = iota
	nodeElement
	nodeText
)

type xmlAttr struct {
	Name      string
	Local     string
	Namespace string
	Value     string
}

type xmlNode struct {
	Type      nodeType
	Name      string
	Local     string
	Namespace string
	Attrs     []xmlAttr
	Children  []*xmlNode
	Parent    *xmlNode
	Text      string
}

func newElement(name string) *xmlNode {
	local := name
	if idx := strings.Index(name, ":"); idx >= 0 {
		local = name[idx+1:]
	}
	return &xmlNode{Type: nodeElement, Name: name, Local: local}
}

func newText(text string) *xmlNode {
	return &xmlNode{Type: nodeText, Text: text}
}

func (n *xmlNode) appendChild(child *xmlNode) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

func (n *xmlNode) setAttr(name, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	local := name
	if idx := strings.Index(name, ":"); idx >= 0 {
		local = name[idx+1:]
	}
	n.Attrs = append(n.Attrs, xmlAttr{
		Name:  name,
		Local: local,
		Value: value,
	})
}

func (n *xmlNode) attrValue(name string) (string, bool) {
	for _, attr := range n.Attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (n *xmlNode) attrValueLocal(local string) (string, bool) {
	for _, attr := range n.Attrs {
		if attr.Local == local && attr.Namespace == "" {
			return attr.Value, true
		}
	}
	return "", false
}

func (n *xmlNode) attrValueNS(namespace, local, qualified string) (string, bool) {
	if qualified != "" {
		if v, ok := n.attrValue(qualified); ok {
			return v, true
		}
	}
	if namespace == "" {
		return n.attrValueLocal(local)
	}
	for _, attr := range n.Attrs {
		if attr.Local == local && attr.Namespace == namespace {
			return attr.Value, true
		}
	}
	return "", false
}

func (n *xmlNode) hasAttrLocal(local string) bool {
	_, ok := n.attrValueLocal(local)
	return ok
}

func (n *xmlNode) textContent() string {
	if n.Type == nodeText {
		return n.Text
	}
	var sb strings.Builder
	var walk func(node *xmlNode)
	walk = func(node *xmlNode) {
		if node.Type == nodeText {
			sb.WriteString(node.Text)
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(n)
	return sb.String()
}

func (n *xmlNode) innerXML() string {
	var sb strings.Builder
	for _, child := range n.Children {
		serializeNode(&sb, child, false, 0)
	}
	return sb.String()
}

func parseXMLDocument(input string) (*xmlNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(input))
	doc := &xmlNode{Type: nodeDocument}

	stack := []*xmlNode{doc}
	nsStack := []map[string]string{{
		"xml": nsXML,
	}}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			parent := stack[len(stack)-1]

			parentNS := nsStack[len(nsStack)-1]
			currNS := make(map[string]string, len(parentNS))
			for k, v := range parentNS {
				currNS[k] = v
			}

			for _, attr := range t.Attr {
				if isNamespaceDecl(attr) {
					prefix := attr.Name.Local
					if prefix == "xmlns" {
						prefix = ""
					}
					if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
						prefix = ""
					}
					currNS[prefix] = attr.Value
				}
			}

			nsStack = append(nsStack, currNS)

			prefix := prefixForURI(t.Name.Space, currNS)
			qualified := qualifyName(prefix, t.Name.Local)

			node := &xmlNode{
				Type:      nodeElement,
				Name:      qualified,
				Local:     t.Name.Local,
				Namespace: t.Name.Space,
			}

			for _, attr := range t.Attr {
				if isNamespaceDecl(attr) {
					continue
				}
				attrPrefix := prefixForURI(attr.Name.Space, currNS)
				attrQualified := qualifyName(attrPrefix, attr.Name.Local)
				node.Attrs = append(node.Attrs, xmlAttr{
					Name:      attrQualified,
					Local:     attr.Name.Local,
					Namespace: attr.Name.Space,
					Value:     attr.Value,
				})
			}

			parent.appendChild(node)
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			if len(nsStack) > 1 {
				nsStack = nsStack[:len(nsStack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			parent := stack[len(stack)-1]
			text := string([]byte(t))
			if text == "" {
				continue
			}
			if len(parent.Children) > 0 {
				last := parent.Children[len(parent.Children)-1]
				if last.Type == nodeText {
					last.Text += text
					continue
				}
			}
			parent.appendChild(&xmlNode{Type: nodeText, Text: text})
		}
	}
	return doc, nil
}

func isNamespaceDecl(attr xml.Attr) bool {
	if attr.Name.Space == "xmlns" {
		return true
	}
	if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
		return true
	}
	return false
}

func prefixForURI(uri string, scope map[string]string) string {
	if uri == "" {
		return ""
	}
	for prefix, space := range scope {
		if space == uri {
			return prefix
		}
	}
	return ""
}

func qualifyName(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

func serializeNode(sb *strings.Builder, node *xmlNode, pretty bool, depth int) {
	switch node.Type {
	case nodeDocument:
		for _, child := range node.Children {
			serializeNode(sb, child, pretty, depth)
		}
	case nodeText:
		if pretty && strings.TrimSpace(node.Text) == "" {
			return
		}
		sb.WriteString(escapeText(node.Text))
	case nodeElement:
		sb.WriteString("<")
		sb.WriteString(node.Name)
		for _, attr := range node.Attrs {
			sb.WriteString(" ")
			sb.WriteString(attr.Name)
			sb.WriteString(`="`)
			sb.WriteString(escapeAttr(attr.Value))
			sb.WriteString(`"`)
		}
		if len(node.Children) == 0 {
			sb.WriteString("/>")
			return
		}
		sb.WriteString(">")

		indent := pretty && shouldIndent(node)
		if indent {
			sb.WriteString("\n")
		}
		for _, child := range node.Children {
			if indent {
				sb.WriteString(strings.Repeat("  ", depth+1))
			}
			serializeNode(sb, child, pretty, depth+1)
			if indent {
				sb.WriteString("\n")
			}
		}
		if indent {
			sb.WriteString(strings.Repeat("  ", depth))
		}
		sb.WriteString("</")
		sb.WriteString(node.Name)
		sb.WriteString(">")
	}
}

func shouldIndent(node *xmlNode) bool {
	hasElement := false
	for _, child := range node.Children {
		if child.Type == nodeElement {
			hasElement = true
		}
		if child.Type == nodeText {
			if strings.TrimSpace(child.Text) != "" {
				// Significant text content disables indentation to avoid changing mixed content.
				return false
			}
		}
	}
	return hasElement
}

func escapeText(input string) string {
	if input == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
	)
	return replacer.Replace(input)
}

func escapeAttr(input string) string {
	if input == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		`"`, "&quot;",
	)
	return replacer.Replace(input)
}

func findElementsByPath(root *xmlNode, path []string) []*xmlNode {
	if root == nil || len(path) == 0 {
		return nil
	}
	var result []*xmlNode

	var walk func(node *xmlNode)
	walk = func(node *xmlNode) {
		if node.Type == nodeDocument {
			for _, child := range node.Children {
				walk(child)
			}
			return
		}
		if node.Type != nodeElement {
			return
		}
		if nameMatches(node, path[0]) {
			result = append(result, findFrom(node, path[1:])...)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return result
}

func findFrom(node *xmlNode, path []string) []*xmlNode {
	if len(path) == 0 {
		return []*xmlNode{node}
	}
	var result []*xmlNode
	for _, child := range node.Children {
		if child.Type == nodeElement && nameMatches(child, path[0]) {
			result = append(result, findFrom(child, path[1:])...)
		}
	}
	return result
}

func findAllElements(root *xmlNode) []*xmlNode {
	var result []*xmlNode
	var walk func(node *xmlNode)
	walk = func(node *xmlNode) {
		if node.Type == nodeElement {
			result = append(result, node)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return result
}

func findDescendantElements(root *xmlNode, match func(*xmlNode) bool) []*xmlNode {
	var result []*xmlNode
	var walk func(node *xmlNode)
	walk = func(node *xmlNode) {
		if node.Type == nodeElement && match(node) {
			result = append(result, node)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, child := range root.Children {
		walk(child)
	}
	return result
}

func hasDescendantTag(root *xmlNode, tag string) bool {
	var found bool
	var walk func(node *xmlNode)
	walk = func(node *xmlNode) {
		if found {
			return
		}
		if node.Type == nodeElement && nameMatches(node, tag) {
			found = true
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, child := range root.Children {
		walk(child)
	}
	return found
}

func nameMatches(node *xmlNode, name string) bool {
	if node.Name == name {
		return true
	}
	return node.Local == name
}
