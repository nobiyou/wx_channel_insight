package ingest

import (
	"html"
	"strings"
)

// sanitizeWXText turns wx_channel's highlighted HTML text into plain text at
// the ingestion boundary, so every downstream read model shares the same value.
func sanitizeWXText(value string) string {
	value = stripWXHTMLTags(value)
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func stripWXHTMLTags(value string) string {
	var plain strings.Builder
	plain.Grow(len(value))

	for index := 0; index < len(value); {
		if value[index] != '<' {
			plain.WriteByte(value[index])
			index++
			continue
		}

		if strings.HasPrefix(value[index:], "<!--") {
			end := strings.Index(value[index+4:], "-->")
			if end >= 0 {
				index += end + 7
				continue
			}
		}

		end, tagName, ok := scanWXHTMLTag(value, index)
		if !ok {
			plain.WriteByte(value[index])
			index++
			continue
		}
		if isWXBlockTag(tagName) {
			plain.WriteByte(' ')
		}
		index = end
	}

	return plain.String()
}

func scanWXHTMLTag(value string, start int) (int, string, bool) {
	if start+1 >= len(value) {
		return 0, "", false
	}

	index := start + 1
	if value[index] == '/' {
		index++
	}
	if index >= len(value) || !isWXHTMLNameStart(value[index]) {
		return 0, "", false
	}

	nameStart := index
	for index < len(value) && isWXHTMLNameChar(value[index]) {
		index++
	}
	tagName := strings.ToLower(value[nameStart:index])

	var quote byte
	for ; index < len(value); index++ {
		current := value[index]
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case '>':
			return index + 1, tagName, true
		}
	}

	return 0, "", false
}

func isWXHTMLNameStart(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func isWXHTMLNameChar(value byte) bool {
	return isWXHTMLNameStart(value) || (value >= '0' && value <= '9')
}

func isWXBlockTag(tagName string) bool {
	switch tagName {
	case "article", "blockquote", "br", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "ol", "p", "section", "ul":
		return true
	default:
		return false
	}
}
