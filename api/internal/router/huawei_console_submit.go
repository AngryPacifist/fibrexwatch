package router

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var queryFragment = regexp.MustCompile(`^[&?][A-Za-z0-9_.=&/-]+$`)

const maxSubmitterBytes = 20000

// submitterSignals mark a function that sends changes to the router: the
// console builds a form of submit parameters, points it at a .cgi endpoint
// and posts it, or issues the same request through its AJAX helpers.
var submitterSignals = []string{"addparameter", "setaction", ".cgi", ".submit(", "ajax"}

// submitFunctionBodies returns the source of every function whose body
// contains a submitter signal. The router renders SSIDs, keys and PINs into
// the same script, so string literals are masked unless they are structure
// the page already declares (control ids and names, submit-field keys, .cgi
// endpoints, TR-069 object paths), and numbers of five or more digits are
// masked as well.
func submitFunctionBodies(document string, safeLiterals map[string]struct{}) map[string]string {
	bodies := map[string]string{}
	for _, match := range javascriptFunction.FindAllStringSubmatchIndex(document, -1) {
		name := document[match[2]:match[3]]
		open := strings.IndexByte(document[match[1]:], '{')
		if open < 0 {
			continue
		}
		start := match[1] + open
		body := functionBody(document[start:])
		lower := strings.ToLower(body)
		signalled := false
		for _, signal := range submitterSignals {
			if strings.Contains(lower, signal) {
				signalled = true
				break
			}
		}
		if !signalled {
			continue
		}
		// Pages redefine some helpers under the same name; each definition is
		// kept under a numbered key so a later one cannot hide an earlier one.
		key := name
		for copy := 2; ; copy++ {
			if _, taken := bodies[key]; !taken {
				break
			}
			key = name + "#" + strconv.Itoa(copy)
		}
		bodies[key] = maskJavaScript(body, safeLiterals)
	}
	return bodies
}

// functionBody returns the text from an opening brace to its matching close,
// skipping braces inside strings and comments. An unbalanced body is cut at
// maxSubmitterBytes so one malformed script cannot swallow the page.
func functionBody(source string) string {
	depth := 0
	var quote byte
	for index := 0; index < len(source) && index < maxSubmitterBytes; index++ {
		char := source[index]
		switch {
		case quote != 0:
			if char == '\\' {
				index++
			} else if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"' || char == '`':
			quote = char
		case char == '/' && index+1 < len(source) && source[index+1] == '/':
			if end := strings.IndexByte(source[index:], '\n'); end >= 0 {
				index += end
			} else {
				return source[:index]
			}
		case char == '/' && index+1 < len(source) && source[index+1] == '*':
			if end := strings.Index(source[index+2:], "*/"); end >= 0 {
				index += end + 3
			} else {
				return source[:index]
			}
		case char == '{':
			depth++
		case char == '}':
			depth--
			if depth == 0 {
				return source[:index+1]
			}
		}
	}
	if len(source) > maxSubmitterBytes {
		return source[:maxSubmitterBytes]
	}
	return source
}

func maskJavaScript(source string, safeLiterals map[string]struct{}) string {
	var out strings.Builder
	for index := 0; index < len(source); {
		char := source[index]
		switch {
		case char == '\'' || char == '"' || char == '`':
			end := index + 1
			for end < len(source) && source[end] != char {
				if source[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(source) {
				end = len(source) - 1
			}
			literal := source[index+1 : end]
			out.WriteByte(char)
			if char != '`' && safeLiteral(literal, safeLiterals) {
				out.WriteString(literal)
			} else {
				out.WriteString("<s>")
			}
			out.WriteByte(char)
			index = end + 1
		case char >= '0' && char <= '9' && (index == 0 || !isIdentifierByte(source[index-1])):
			end := index
			for end < len(source) && source[end] >= '0' && source[end] <= '9' {
				end++
			}
			if end-index >= 5 {
				out.WriteString("<n>")
			} else {
				out.WriteString(source[index:end])
			}
			index = end
		default:
			out.WriteByte(char)
			index++
		}
	}
	return out.String()
}

func safeLiteral(literal string, safeLiterals map[string]struct{}) bool {
	if _, ok := safeLiterals[literal]; ok {
		return true
	}
	if submitFieldKey.MatchString(literal) || strings.HasPrefix(literal, "InternetGatewayDevice.") {
		return true
	}
	if strings.Contains(literal, ".cgi") && !strings.ContainsAny(literal, " <>") {
		return true
	}
	// Action URLs are assembled from fragments such as
	// "&RequestFile=html/amp/wlanbasic/WlanBasic.asp".
	if queryFragment.MatchString(literal) {
		return true
	}
	for _, r := range literal {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return len(literal) <= 3
}

func isIdentifierByte(char byte) bool {
	return char == '_' || char == '$' || char == '.' || char >= '0' && char <= '9' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}

func pageControlNames(document string) map[string]struct{} {
	names := map[string]struct{}{}
	tags := htmlFormTag.FindAllStringSubmatch(document, -1)
	for _, match := range htmlControlTag.FindAllStringSubmatch(document, -1) {
		tags = append(tags, []string{match[0], match[2]})
	}
	for _, tag := range tags {
		for _, attribute := range htmlStructuralAttribute.FindAllStringSubmatch(tag[1], -1) {
			kind := strings.ToLower(attribute[1])
			if (kind == "id" || kind == "name") && attribute[2] != "" {
				names[attribute[2]] = struct{}{}
			}
		}
	}
	return names
}
