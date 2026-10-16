package router

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

var htmlFormTag = regexp.MustCompile(`(?is)<form\b([^>]*)>`)
var htmlControlTag = regexp.MustCompile(`(?is)<(input|select|textarea|button)\b([^>]*)>`)
var htmlStructuralAttribute = regexp.MustCompile(`(?i)\b(id|name|type|action|method)\s*=\s*["']([^"']*)["']`)
var javascriptConstructor = regexp.MustCompile(`new\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
var submitFieldKey = regexp.MustCompile(`^[a-zA-Z]\.[A-Za-z_][A-Za-z0-9_.]*$`)

var builtinConstructors = map[string]struct{}{
	"Array": {}, "Date": {}, "RegExp": {}, "Object": {}, "String": {}, "Function": {},
	"Image": {}, "Option": {}, "Error": {}, "Number": {}, "Boolean": {}, "XMLHttpRequest": {},
}

// ConsoleScan lists the structure of every console page reachable from the
// authenticated menu. Router-rendered values (SSIDs, keys, addresses) live in
// string literals and value attributes, so neither is copied: only attribute
// names, endpoint and object-path literals, submit-field keys, constructor
// names, function signatures and the masked bodies of submitting functions
// are kept.
type ConsoleScan struct {
	LandingPath string        `json:"landingPath"`
	Menu        [][]string    `json:"menu"`
	Pages       []ConsolePage `json:"pages"`
	Missing     []string      `json:"missing,omitempty"`
	Stopped     string        `json:"stopped,omitempty"`
}

type ConsolePage struct {
	Path        string            `json:"path"`
	Status      string            `json:"status"`
	Bytes       int               `json:"bytes,omitempty"`
	Forms       []string          `json:"forms,omitempty"`
	Controls    []string          `json:"controls,omitempty"`
	Endpoints   []string          `json:"endpoints,omitempty"`
	FieldKeys   []string          `json:"fieldKeys,omitempty"`
	ObjectPaths []string          `json:"objectPaths,omitempty"`
	Records     map[string]int    `json:"records,omitempty"`
	Functions   []string          `json:"functions,omitempty"`
	Submitters  map[string]string `json:"submitters,omitempty"`
	References  []string          `json:"references,omitempty"`
}

// ScanConsole issues GET requests only. .cgi endpoints are where the console
// applies changes, so they are recorded as references and never requested.
func (h *HuaweiAdapter) ScanConsole(ctx context.Context, maxPages int, pause time.Duration) (ConsoleScan, error) {
	if err := h.Authenticate(ctx); err != nil {
		return ConsoleScan{}, err
	}
	scan := ConsoleScan{LandingPath: h.landingPath}
	queue := make([]string, 0)
	queued := map[string]struct{}{}
	enqueue := func(paths []string) {
		for _, path := range paths {
			if !scannablePage(path) {
				continue
			}
			if _, seen := queued[path]; seen {
				continue
			}
			queued[path] = struct{}{}
			queue = append(queue, path)
		}
	}
	enqueue(uniqueAssetPaths(h.landingBody))
	enqueue(referencedLocalPages(h.landingBody, h.landingPath))
	scan.Menu = menuEntries(h.landingBody)
	enqueue(extractMenuPaths(h.landingBody))
	menu, err := h.fetchPage(ctx, "/Cusjs/frame.asp")
	if err == nil {
		scan.Menu = append(scan.Menu, menuEntries(menu)...)
		enqueue(extractMenuPaths(menu))
		enqueue(referencedLocalPages(menu, "/Cusjs/frame.asp"))
	}
	consecutiveAuthFailures := 0
	for index := 0; index < len(queue) && index < maxPages; index++ {
		if ctx.Err() != nil {
			scan.Stopped = "context cancelled"
			break
		}
		path := queue[index]
		body, err := h.fetchPage(ctx, path)
		page := ConsolePage{Path: path, Status: "ok"}
		switch {
		case errors.Is(err, ErrAuthenticationFailed):
			page.Status = "denied"
			consecutiveAuthFailures++
		case err != nil && strings.HasSuffix(err.Error(), "status 404"):
			scan.Missing = append(scan.Missing, path)
			time.Sleep(pause)
			continue
		case err != nil:
			page.Status = err.Error()
		default:
			consecutiveAuthFailures = 0
			describeConsolePage(&page, body)
			enqueue(page.References)
		}
		scan.Pages = append(scan.Pages, page)
		// A run of denials usually means the session ended rather than that
		// each page is restricted. Logging in again from here would spend
		// attempts against the firmware's three-failure lockout.
		if consecutiveAuthFailures >= 10 {
			scan.Stopped = "ten consecutive denied pages; session likely ended"
			break
		}
		time.Sleep(pause)
	}
	return scan, nil
}

func scannablePage(path string) bool {
	lower := strings.ToLower(path)
	if !strings.HasPrefix(lower, "/") || strings.Contains(lower, "logout") || strings.Contains(lower, "login") {
		return false
	}
	return strings.HasSuffix(lower, ".asp") || strings.HasSuffix(lower, ".html")
}

func menuEntries(document string) [][]string {
	entries := make([][]string, 0)
	for _, match := range menuDataCall.FindAllStringSubmatch(document, -1) {
		if len(match) != 2 {
			continue
		}
		args := splitJavaScriptArgs(match[1])
		entry := make([]string, 0, len(args))
		for _, arg := range args {
			entry = append(entry, parseHuaweiString(arg))
		}
		entries = append(entries, entry)
	}
	return entries
}

func describeConsolePage(page *ConsolePage, document string) {
	page.Bytes = len(document)
	for _, match := range htmlFormTag.FindAllStringSubmatch(document, -1) {
		page.Forms = append(page.Forms, structuralAttributes(match[1]))
	}
	for _, match := range htmlControlTag.FindAllStringSubmatch(document, -1) {
		page.Controls = append(page.Controls, strings.ToLower(match[1])+" "+structuralAttributes(match[2]))
	}
	page.Controls = uniqueStrings(page.Controls)
	endpoints, fieldKeys, objectPaths := []string{}, []string{}, []string{}
	for _, match := range javascriptStringLiteral.FindAllStringSubmatch(document, -1) {
		literal := strings.TrimSpace(match[1])
		if len(literal) > 300 {
			continue
		}
		switch {
		case strings.Contains(literal, ".cgi"):
			endpoints = append(endpoints, literal)
		case strings.HasPrefix(literal, "InternetGatewayDevice."):
			objectPaths = append(objectPaths, literal)
		case submitFieldKey.MatchString(literal):
			fieldKeys = append(fieldKeys, literal)
		}
	}
	page.Endpoints, page.FieldKeys, page.ObjectPaths = sortedUnique(endpoints), sortedUnique(fieldKeys), sortedUnique(objectPaths)
	page.Records = map[string]int{}
	for _, match := range javascriptConstructor.FindAllStringSubmatch(document, -1) {
		if _, builtin := builtinConstructors[match[1]]; !builtin {
			page.Records[match[1]]++
		}
	}
	functions := []string{}
	for _, match := range javascriptFunction.FindAllStringSubmatch(document, -1) {
		functions = append(functions, match[1]+"("+strings.Join(strings.Fields(match[2]), "")+")")
	}
	page.Functions = sortedUnique(functions)
	page.Submitters = submitFunctionBodies(document, pageControlNames(document))
	// The menu script lists pages relative to the site root while page
	// scripts use paths relative to their own directory; the text alone does
	// not say which, so both resolutions are kept and the wrong one 404s.
	page.References = uniqueStrings(append(referencedLocalPages(document, page.Path), referencedLocalPages(document, "/")...))
}

func structuralAttributes(tag string) string {
	attributes := make([]string, 0)
	for _, match := range htmlStructuralAttribute.FindAllStringSubmatch(tag, -1) {
		attributes = append(attributes, strings.ToLower(match[1])+"="+match[2])
	}
	sort.Strings(attributes)
	return strings.Join(attributes, " ")
}

func sortedUnique(values []string) []string {
	result := uniqueStrings(values)
	sort.Strings(result)
	return result
}
