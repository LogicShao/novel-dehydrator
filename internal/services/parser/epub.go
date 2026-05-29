package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"golang.org/x/net/html"
)

type EPUBBook struct {
	Title        string
	Author       string
	Chapters     []EPUBChapter
	HasNestedTOC bool
}

type EPUBChapter struct {
	Title  string
	Text   string
	Volume string
	Href   string
}

type containerXML struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type packageXML struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Entries []metadataEntry `xml:",any"`
	} `xml:"metadata"`
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		TOC   string `xml:"toc,attr"`
		Items []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

type ncxXML struct {
	NavMap []ncxNavPoint `xml:"navMap>navPoint"`
}

type ncxNavPoint struct {
	Label struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []ncxNavPoint `xml:"navPoint"`
}

type tocEntry struct {
	Title    string
	Href     string
	Children []tocEntry
}

type metadataEntry struct {
	XMLName xml.Name
	Text    string
}

func (m *metadataEntry) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	m.XMLName = start.Name
	var text string
	if err := decoder.DecodeElement(&text, &start); err != nil {
		return err
	}
	m.Text = strings.TrimSpace(text)
	return nil
}

type manifestItem struct {
	ID         string
	Href       string
	MediaType  string
	Properties string
	AbsPath    string
}

func ParseEPUB(filePath string) (*EPUBBook, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open epub: %w", err)
	}
	defer reader.Close()

	return parseEPUBFromZip(&reader.Reader)
}

func parseEPUBFromZip(reader *zip.Reader) (*EPUBBook, error) {
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[file.Name] = file
	}

	opfPath, err := parseContainer(files)
	if err != nil {
		return nil, err
	}

	pkg, manifestByID, manifestByPath, orderedPaths, err := parsePackage(files, opfPath)
	if err != nil {
		return nil, err
	}

	tocEntries, hasNestedTOC := parseTOC(files, path.Dir(opfPath), pkg, manifestByID)
	chapterTitles := flattenTOC(tocEntries, "")

	chapters := make([]EPUBChapter, 0, len(orderedPaths))
	seen := make(map[string]struct{}, len(orderedPaths))
	for _, chapterPath := range orderedPaths {
		item, ok := manifestByPath[chapterPath]
		if !ok {
			continue
		}

		titleInfo := chapterTitles[chapterPath]
		title := titleInfo.Title
		if title == "" {
			title = fallbackTitle(item.Href)
		}

		key := title + "\x00" + titleInfo.Volume
		if _, exists := seen[key]; exists {
			continue
		}

		text, err := extractHTMLText(files, chapterPath)
		if err != nil {
			return nil, fmt.Errorf("extract text from %s: %w", chapterPath, err)
		}

		seen[key] = struct{}{}
		chapters = append(chapters, EPUBChapter{
			Title:  title,
			Text:   text,
			Volume: titleInfo.Volume,
			Href:   item.Href,
		})
	}

	return &EPUBBook{
		Title:        firstNonEmpty(metadataValue(pkg.Metadata.Entries, "title"), "未知书名"),
		Author:       metadataValue(pkg.Metadata.Entries, "creator"),
		Chapters:     chapters,
		HasNestedTOC: hasNestedTOC,
	}, nil
}

func parseContainer(files map[string]*zip.File) (string, error) {
	data, err := readZipFile(files, "META-INF/container.xml")
	if err != nil {
		return "", fmt.Errorf("read container.xml: %w", err)
	}

	var container containerXML
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", fmt.Errorf("parse container.xml: %w", err)
	}
	if len(container.Rootfiles) == 0 || strings.TrimSpace(container.Rootfiles[0].FullPath) == "" {
		return "", fmt.Errorf("container.xml missing OPF rootfile")
	}

	return path.Clean(container.Rootfiles[0].FullPath), nil
}

func parsePackage(files map[string]*zip.File, opfPath string) (*packageXML, map[string]manifestItem, map[string]manifestItem, []string, error) {
	data, err := readZipFile(files, opfPath)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("read OPF: %w", err)
	}

	var pkg packageXML
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse OPF: %w", err)
	}

	baseDir := path.Dir(opfPath)
	manifestByID := make(map[string]manifestItem, len(pkg.Manifest))
	manifestByPath := make(map[string]manifestItem, len(pkg.Manifest))
	for _, item := range pkg.Manifest {
		absPath := resolveOPFPath(baseDir, item.Href)
		manifest := manifestItem{
			ID:         item.ID,
			Href:       item.Href,
			MediaType:  item.MediaType,
			Properties: item.Properties,
			AbsPath:    absPath,
		}
		manifestByID[item.ID] = manifest
		manifestByPath[absPath] = manifest
	}

	orderedPaths := make([]string, 0, len(pkg.Spine.Items))
	for _, itemRef := range pkg.Spine.Items {
		manifest, ok := manifestByID[itemRef.IDRef]
		if !ok {
			continue
		}
		orderedPaths = append(orderedPaths, manifest.AbsPath)
	}

	return &pkg, manifestByID, manifestByPath, orderedPaths, nil
}

func parseTOC(files map[string]*zip.File, opfDir string, pkg *packageXML, manifestByID map[string]manifestItem) ([]tocEntry, bool) {
	if navPath := findNavPath(manifestByID); navPath != "" {
		entries, hasNested, err := parseNavTOC(files, navPath)
		if err == nil && len(entries) > 0 {
			return entries, hasNested
		}
	}

	if ncxPath := findNCXPath(opfDir, pkg, manifestByID); ncxPath != "" {
		entries, hasNested, err := parseNCXTOC(files, ncxPath)
		if err == nil && len(entries) > 0 {
			return entries, hasNested
		}
	}

	return nil, false
}

func findNavPath(manifestByID map[string]manifestItem) string {
	for _, item := range manifestByID {
		if strings.Contains(item.Properties, "nav") {
			return item.AbsPath
		}
	}
	for _, item := range manifestByID {
		if item.MediaType == "application/xhtml+xml" && strings.Contains(strings.ToLower(item.Href), "nav") {
			return item.AbsPath
		}
	}
	return ""
}

func findNCXPath(opfDir string, pkg *packageXML, manifestByID map[string]manifestItem) string {
	if tocID := strings.TrimSpace(pkg.Spine.TOC); tocID != "" {
		if item, ok := manifestByID[tocID]; ok {
			return item.AbsPath
		}
	}
	for _, item := range manifestByID {
		if item.MediaType == "application/x-dtbncx+xml" {
			return item.AbsPath
		}
	}
	for _, item := range pkg.Manifest {
		if strings.HasSuffix(strings.ToLower(item.Href), ".ncx") {
			return resolveOPFPath(opfDir, item.Href)
		}
	}
	return ""
}

func parseNCXTOC(files map[string]*zip.File, ncxPath string) ([]tocEntry, bool, error) {
	data, err := readZipFile(files, ncxPath)
	if err != nil {
		return nil, false, err
	}

	var ncx ncxXML
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil, false, err
	}

	entries := make([]tocEntry, 0, len(ncx.NavMap))
	hasNested := false
	for _, point := range ncx.NavMap {
		entry := buildNCXEntry(path.Dir(ncxPath), point)
		if len(entry.Children) > 0 {
			hasNested = true
		}
		entries = append(entries, entry)
	}

	return entries, hasNested, nil
}

func buildNCXEntry(baseDir string, point ncxNavPoint) tocEntry {
	entry := tocEntry{
		Title: strings.TrimSpace(point.Label.Text),
		Href:  normalizeTOCHref(baseDir, point.Content.Src),
	}
	for _, child := range point.Children {
		entry.Children = append(entry.Children, buildNCXEntry(baseDir, child))
	}
	return entry
}

func parseNavTOC(files map[string]*zip.File, navPath string) ([]tocEntry, bool, error) {
	data, err := readZipFile(files, navPath)
	if err != nil {
		return nil, false, err
	}

	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}

	navNode := findNavTOCNode(doc)
	if navNode == nil {
		return nil, false, fmt.Errorf("nav toc not found")
	}

	listNode := firstChildElement(navNode, "ol")
	if listNode == nil {
		listNode = firstChildElement(navNode, "ul")
	}
	if listNode == nil {
		return nil, false, fmt.Errorf("nav list not found")
	}

	entries := parseNavList(path.Dir(navPath), listNode)
	return entries, hasNestedEntries(entries), nil
}

func parseNavList(baseDir string, listNode *html.Node) []tocEntry {
	entries := make([]tocEntry, 0)
	for child := listNode.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "li" {
			continue
		}

		entry := tocEntry{}
		for node := child.FirstChild; node != nil; node = node.NextSibling {
			if node.Type != html.ElementNode {
				continue
			}
			switch node.Data {
			case "a":
				entry.Title = strings.TrimSpace(nodeText(node))
				entry.Href = normalizeTOCHref(baseDir, attr(node, "href"))
			case "span":
				if entry.Title == "" {
					entry.Title = strings.TrimSpace(nodeText(node))
				}
			case "ol", "ul":
				entry.Children = append(entry.Children, parseNavList(baseDir, node)...)
			}
		}
		if entry.Title != "" || entry.Href != "" || len(entry.Children) > 0 {
			entries = append(entries, entry)
		}
	}
	return entries
}

type tocTitle struct {
	Title  string
	Volume string
}

func flattenTOC(entries []tocEntry, volume string) map[string]tocTitle {
	flat := make(map[string]tocTitle)
	var walk func([]tocEntry, string)
	walk = func(nodes []tocEntry, parentVolume string) {
		for _, entry := range nodes {
			currentVolume := parentVolume
			if entry.Href == "" && len(entry.Children) > 0 {
				currentVolume = strings.TrimSpace(entry.Title)
			}
			if entry.Href != "" {
				flat[stripFragment(entry.Href)] = tocTitle{
					Title:  firstNonEmpty(strings.TrimSpace(entry.Title), fallbackTitle(entry.Href)),
					Volume: currentVolume,
				}
			}
			if len(entry.Children) > 0 {
				walk(entry.Children, currentVolume)
			}
		}
	}
	walk(entries, volume)
	return flat
}

func extractHTMLText(files map[string]*zip.File, filePath string) (string, error) {
	data, err := readZipFile(files, filePath)
	if err != nil {
		return "", err
	}

	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	var blocks []string
	var textBuilder strings.Builder
	flushText := func() {
		text := normalizeWhitespace(textBuilder.String())
		if text != "" {
			blocks = append(blocks, text)
		}
		textBuilder.Reset()
	}

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style") {
			return
		}
		if node.Type == html.TextNode {
			textBuilder.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			flushText()
			return
		}
		if node.Type == html.ElementNode && isBlockTextNode(node.Data) {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			flushText()
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	flushText()
	if len(blocks) == 0 {
		text := strings.TrimSpace(nodeText(doc))
		if text != "" {
			blocks = append(blocks, normalizeWhitespace(text))
		}
	}

	return strings.Join(deduplicateAdjacent(blocks), "\n\n"), nil
}

func resolveOPFPath(opfDir, href string) string {
	href = path.Clean(strings.TrimSpace(href))
	if href == "" || href == "." {
		return ""
	}
	if opfDir == "" || opfDir == "." {
		return href
	}
	if href == opfDir || strings.HasPrefix(href, opfDir+"/") {
		return href
	}
	return path.Clean(path.Join(opfDir, href))
}

func readZipFile(files map[string]*zip.File, name string) ([]byte, error) {
	file, ok := files[path.Clean(name)]
	if !ok {
		return nil, os.ErrNotExist
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func normalizeTOCHref(baseDir, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	parts := strings.SplitN(href, "#", 2)
	return path.Clean(path.Join(baseDir, parts[0]))
}

func stripFragment(href string) string {
	return strings.SplitN(href, "#", 2)[0]
}

func fallbackTitle(href string) string {
	base := path.Base(stripFragment(href))
	base = strings.TrimSuffix(base, path.Ext(base))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.TrimSpace(base)
	if base == "" {
		return "（无标题）"
	}
	return base
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func metadataValue(entries []metadataEntry, wantLocal string) string {
	for _, entry := range entries {
		if !metadataNameMatches(entry.XMLName, wantLocal) {
			continue
		}
		if entry.Text != "" {
			return entry.Text
		}
	}
	return ""
}

func metadataNameMatches(name xml.Name, wantLocal string) bool {
	if strings.EqualFold(strings.TrimSpace(name.Local), wantLocal) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(name.Space), "dc") && strings.EqualFold(strings.TrimSpace(name.Local), wantLocal) {
		return true
	}
	local := strings.TrimSpace(name.Local)
	if strings.EqualFold(local, "dc:"+wantLocal) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(name.Space), "http://purl.org/dc/elements/1.1/") && strings.EqualFold(local, wantLocal)
}

func findNavTOCNode(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && node.Data == "nav" {
		epubType := attr(node, "epub:type")
		if strings.Contains(epubType, "toc") || strings.Contains(attr(node, "type"), "toc") {
			return node
		}
		if strings.Contains(strings.ToLower(attr(node, "role")), "doc-toc") {
			return node
		}
		if strings.Contains(strings.ToLower(attr(node, "id")), "toc") {
			return node
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findNavTOCNode(child); found != nil {
			return found
		}
	}
	return nil
}

func firstChildElement(node *html.Node, names ...string) *html.Node {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if _, ok := allowed[child.Data]; ok {
			return child
		}
	}
	return nil
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		if current.Type == html.ElementNode && current.Data == "br" {
			builder.WriteString("\n")
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func isBlockTextNode(name string) bool {
	switch name {
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li":
		return true
	default:
		return false
	}
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func deduplicateAdjacent(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) > 0 && result[len(result)-1] == value {
			continue
		}
		result = append(result, value)
	}
	return result
}

func hasNestedEntries(entries []tocEntry) bool {
	for _, entry := range entries {
		if len(entry.Children) > 0 {
			return true
		}
	}
	return false
}
