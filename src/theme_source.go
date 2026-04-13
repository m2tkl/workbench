package taskbench

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var genericSourceDocumentTitlePattern = regexp.MustCompile(`(?i)^slide\s+\d+$`)

type SourceImportOptions struct {
	Title string
	Tags  []string
	Links []string
	Now   time.Time
}

type sourceDocumentConversion struct {
	Body      string
	Converter string
}

func (v VaultFS) ImportSourceDocument(sourcePath string, opts SourceImportOptions) (SourceDocument, error) {
	if err := v.EnsureLayout(); err != nil {
		return SourceDocument{}, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return SourceDocument{}, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return SourceDocument{}, err
	}
	if info.IsDir() {
		return SourceDocument{}, fmt.Errorf("source path must be a file: %s", sourcePath)
	}

	stagedRawPath, err := copyFileUnique(sourcePath, v.SourceStagedDir())
	if err != nil {
		return SourceDocument{}, err
	}
	return v.importSourceDocumentFromStagedPath(stagedRawPath, filepath.Base(sourcePath), opts)
}

func (v VaultFS) ImportStagedSourceDocument(stagedName string, opts SourceImportOptions) (SourceDocument, error) {
	stagedName = filepath.Base(strings.TrimSpace(stagedName))
	if stagedName == "" || stagedName == "." {
		return SourceDocument{}, fmt.Errorf("staged file is required")
	}
	stagedPath := filepath.Join(v.SourceStagedDir(), stagedName)
	if _, err := os.Stat(stagedPath); err != nil {
		if os.IsNotExist(err) {
			return SourceDocument{}, fmt.Errorf("staged file not found: %s", stagedName)
		}
		return SourceDocument{}, err
	}
	return v.importSourceDocumentFromStagedPath(stagedPath, stagedName, opts)
}

func (v VaultFS) StageSourceUpload(filename string, content io.Reader) (string, error) {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." {
		filename = "upload.bin"
	}
	if err := v.EnsureLayout(); err != nil {
		return "", err
	}
	path, err := uniquePath(v.SourceStagedDir(), filename)
	if err != nil {
		return "", err
	}
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, content); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return filepath.Base(path), nil
}

func (v VaultFS) ListStagedSourceFiles() ([]string, error) {
	entries, err := readDirSorted(v.SourceStagedDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

func (v VaultFS) importSourceDocumentFromStagedPath(stagedRawPath, originalFilename string, opts SourceImportOptions) (SourceDocument, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	conversion, err := convertSourceDocument(stagedRawPath)
	if err != nil {
		return SourceDocument{}, err
	}
	convertedRawPath, err := moveFileUnique(stagedRawPath, v.SourceImportedDir())
	if err != nil {
		return SourceDocument{}, err
	}

	title := chooseSourceDocumentTitle(opts.Title, conversion.Body, originalFilename)
	docPath, err := uniquePath(v.SourceDocumentsDir(), filepath.Base(originalFilename))
	if err != nil {
		return SourceDocument{}, err
	}
	relSourceFile, err := filepath.Rel(filepath.Dir(docPath), convertedRawPath)
	if err != nil {
		return SourceDocument{}, err
	}
	doc := SourceDocument{
		Path:       docPath,
		Title:      title,
		Attachment: filepath.ToSlash(relSourceFile),
		Filename:   originalFilename,
		ImportedAt: now.Format(time.RFC3339),
		Converter:  conversion.Converter,
		Tags:       normalizeStrings(opts.Tags),
		Links:      normalizeStrings(opts.Links),
		Body:       normalizeMarkdown(conversion.Body),
	}
	if err := os.WriteFile(docPath, []byte(renderSourceDocument(doc)), 0o644); err != nil {
		return SourceDocument{}, err
	}
	return doc, nil
}

func convertSourceDocument(path string) (sourceDocumentConversion, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		raw, err := os.ReadFile(path)
		if err != nil {
			return sourceDocumentConversion{}, err
		}
		return sourceDocumentConversion{
			Body:      string(raw),
			Converter: "markdown",
		}, nil
	case ".txt", ".text":
		raw, err := os.ReadFile(path)
		if err != nil {
			return sourceDocumentConversion{}, err
		}
		return sourceDocumentConversion{
			Body:      string(raw),
			Converter: "text",
		}, nil
	case ".csv":
		return convertDelimitedFile(path, ',', "csv")
	case ".tsv":
		return convertDelimitedFile(path, '\t', "tsv")
	case ".docx":
		return convertDocxToMarkdown(path)
	case ".pptx":
		return convertPPTXToMarkdown(path)
	case ".xlsx":
		return convertXLSXToMarkdown(path)
	default:
		name := filepath.Base(path)
		body := strings.Join([]string{
			fmt.Sprintf("# %s", displayTitleFromFilename(name)),
			"",
			"Automatic text extraction is not available for this file type yet.",
			"",
			fmt.Sprintf("- Attached file: `%s`", name),
		}, "\n")
		return sourceDocumentConversion{
			Body:      body,
			Converter: "attachment-only",
		}, nil
	}
}

func convertDelimitedFile(path string, comma rune, converter string) (sourceDocumentConversion, error) {
	f, err := os.Open(path)
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.Comma = comma
	rows, err := reader.ReadAll()
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	return sourceDocumentConversion{
		Body:      renderMarkdownTable(filepath.Base(path), rows),
		Converter: converter,
	}, nil
}

func convertDocxToMarkdown(path string) (sourceDocumentConversion, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	defer zr.Close()

	raw, err := readZipFile(zr.File, "word/document.xml")
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	paragraphs, err := extractOOXMLParagraphs(raw)
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	return sourceDocumentConversion{
		Body:      strings.Join(paragraphs, "\n\n"),
		Converter: "docx",
	}, nil
}

func convertPPTXToMarkdown(path string) (sourceDocumentConversion, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	defer zr.Close()

	slideNames := []string{}
	for _, file := range zr.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slideNames = append(slideNames, file.Name)
		}
	}
	slices.SortFunc(slideNames, naturalCompare)

	sections := []string{}
	for i, name := range slideNames {
		raw, err := readZipFile(zr.File, name)
		if err != nil {
			return sourceDocumentConversion{}, err
		}
		paragraphs, err := extractOOXMLParagraphs(raw)
		if err != nil {
			return sourceDocumentConversion{}, err
		}
		section := []string{fmt.Sprintf("## Slide %d", i+1)}
		if len(paragraphs) == 0 {
			section = append(section, "", "_No text extracted._")
		} else {
			section = append(section, "")
			section = append(section, strings.Join(paragraphs, "\n\n"))
		}
		sections = append(sections, strings.Join(section, "\n"))
	}

	return sourceDocumentConversion{
		Body:      strings.Join(sections, "\n\n"),
		Converter: "pptx",
	}, nil
}

func convertXLSXToMarkdown(path string) (sourceDocumentConversion, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	defer zr.Close()

	sharedStrings, err := parseXLSXSharedStrings(zr.File)
	if err != nil {
		return sourceDocumentConversion{}, err
	}
	workbook, err := parseXLSXWorkbook(zr.File)
	if err != nil {
		return sourceDocumentConversion{}, err
	}

	sections := []string{}
	for _, sheet := range workbook {
		raw, err := readZipFile(zr.File, sheet.Path)
		if err != nil {
			return sourceDocumentConversion{}, err
		}
		rows, err := parseXLSXSheet(raw, sharedStrings)
		if err != nil {
			return sourceDocumentConversion{}, err
		}
		section := []string{fmt.Sprintf("## %s", sheet.Name), ""}
		if len(rows) == 0 {
			section = append(section, "_Empty sheet._")
		} else {
			section = append(section, renderMarkdownTable(sheet.Name, rows))
		}
		sections = append(sections, strings.Join(section, "\n"))
	}

	return sourceDocumentConversion{
		Body:      strings.Join(sections, "\n\n"),
		Converter: "xlsx",
	}, nil
}

func parseXLSXSharedStrings(files []*zip.File) ([]string, error) {
	raw, err := readZipFile(files, "xl/sharedStrings.xml")
	if err != nil {
		if errorsIsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type node struct {
		Text string `xml:",chardata"`
	}
	type run struct {
		Text string `xml:"t"`
	}
	type item struct {
		Text string `xml:"t"`
		Runs []run  `xml:"r"`
	}
	type sharedStrings struct {
		Items []item `xml:"si"`
	}

	var doc sharedStrings
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	values := make([]string, 0, len(doc.Items))
	for _, item := range doc.Items {
		if item.Text != "" {
			values = append(values, strings.TrimSpace(item.Text))
			continue
		}
		parts := []string{}
		for _, run := range item.Runs {
			parts = append(parts, run.Text)
		}
		values = append(values, strings.TrimSpace(strings.Join(parts, "")))
	}
	return values, nil
}

type xlsxSheetRef struct {
	Name string
	Path string
}

func parseXLSXWorkbook(files []*zip.File) ([]xlsxSheetRef, error) {
	workbookRaw, err := readZipFile(files, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	relsRaw, err := readZipFile(files, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}

	type rel struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	}
	type relationships struct {
		Items []rel `xml:"Relationship"`
	}
	type sheet struct {
		Name string `xml:"name,attr"`
		ID   string `xml:"id,attr"`
		RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	}
	type workbook struct {
		Sheets []sheet `xml:"sheets>sheet"`
	}

	var rels relationships
	if err := xml.Unmarshal(relsRaw, &rels); err != nil {
		return nil, err
	}
	relPaths := map[string]string{}
	for _, rel := range rels.Items {
		relPaths[rel.ID] = "xl/" + strings.TrimPrefix(rel.Target, "/")
	}

	var doc workbook
	if err := xml.Unmarshal(workbookRaw, &doc); err != nil {
		return nil, err
	}
	sheets := make([]xlsxSheetRef, 0, len(doc.Sheets))
	for _, sheet := range doc.Sheets {
		path := relPaths[sheet.RID]
		if path == "" {
			path = relPaths[sheet.ID]
		}
		if path == "" {
			continue
		}
		sheets = append(sheets, xlsxSheetRef{Name: sheet.Name, Path: path})
	}
	slices.SortFunc(sheets, func(a, b xlsxSheetRef) int {
		return naturalCompare(a.Path, b.Path)
	})
	return sheets, nil
}

func parseXLSXSheet(raw []byte, sharedStrings []string) ([][]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	rowsByIndex := map[int]map[int]string{}
	currentRow := -1
	currentCol := -1
	currentType := ""
	cellValue := ""
	inlineValue := ""

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "row":
				currentRow = parseRowIndex(attrValue(element.Attr, "r"))
				if currentRow < 0 {
					currentRow = len(rowsByIndex)
				}
			case "c":
				currentType = attrValue(element.Attr, "t")
				currentCol = parseCellColumn(attrValue(element.Attr, "r"))
				cellValue = ""
				inlineValue = ""
			case "v":
				var value string
				if err := decoder.DecodeElement(&value, &element); err != nil {
					return nil, err
				}
				cellValue = strings.TrimSpace(value)
			case "t":
				if currentType == "inlineStr" {
					var value string
					if err := decoder.DecodeElement(&value, &element); err != nil {
						return nil, err
					}
					inlineValue += value
				}
			}
		case xml.EndElement:
			if element.Name.Local != "c" || currentRow < 0 || currentCol < 0 {
				continue
			}
			if rowsByIndex[currentRow] == nil {
				rowsByIndex[currentRow] = map[int]string{}
			}
			rowsByIndex[currentRow][currentCol] = resolveXLSXCellValue(currentType, cellValue, inlineValue, sharedStrings)
		}
	}

	if len(rowsByIndex) == 0 {
		return nil, nil
	}
	rowNumbers := make([]int, 0, len(rowsByIndex))
	maxCols := 0
	for row := range rowsByIndex {
		rowNumbers = append(rowNumbers, row)
		for col := range rowsByIndex[row] {
			if col+1 > maxCols {
				maxCols = col + 1
			}
		}
	}
	slices.Sort(rowNumbers)
	rows := make([][]string, 0, len(rowNumbers))
	for _, rowNumber := range rowNumbers {
		row := make([]string, maxCols)
		for col, value := range rowsByIndex[rowNumber] {
			row[col] = strings.TrimSpace(value)
		}
		rows = append(rows, trimTrailingEmptyCells(row))
	}
	return rows, nil
}

func resolveXLSXCellValue(cellType, rawValue, inlineValue string, sharedStrings []string) string {
	switch cellType {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(rawValue))
		if err != nil || index < 0 || index >= len(sharedStrings) {
			return rawValue
		}
		return sharedStrings[index]
	case "inlineStr":
		return inlineValue
	case "b":
		if strings.TrimSpace(rawValue) == "1" {
			return "TRUE"
		}
		return "FALSE"
	default:
		return rawValue
	}
}

func renderMarkdownTable(title string, rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return ""
	}
	normalized := make([][]string, 0, len(rows))
	for _, row := range rows {
		filled := make([]string, width)
		copy(filled, row)
		normalized = append(normalized, filled)
	}
	header := append([]string(nil), normalized[0]...)
	if isRowBlank(header) {
		for i := range header {
			header[i] = fmt.Sprintf("Column %d", i+1)
		}
	}
	var b strings.Builder
	writeMarkdownTableRow(&b, header)
	writeMarkdownTableRow(&b, repeatString("---", width))
	for _, row := range normalized[1:] {
		writeMarkdownTableRow(&b, row)
	}
	if len(normalized) == 1 {
		writeMarkdownTableRow(&b, repeatString("", width))
	}
	return strings.TrimSpace(b.String())
}

func writeMarkdownTableRow(b *strings.Builder, row []string) {
	b.WriteString("|")
	for _, cell := range row {
		b.WriteString(" ")
		b.WriteString(escapeMarkdownTableCell(cell))
		b.WriteString(" |")
	}
	b.WriteString("\n")
}

func escapeMarkdownTableCell(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func repeatString(value string, count int) []string {
	items := make([]string, count)
	for i := range items {
		items[i] = value
	}
	return items
}

func isRowBlank(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func trimTrailingEmptyCells(row []string) []string {
	last := len(row)
	for last > 0 && strings.TrimSpace(row[last-1]) == "" {
		last--
	}
	return row[:last]
}

func extractOOXMLParagraphs(raw []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	paragraphs := []string{}
	texts := []string{}

	flush := func() {
		joined := strings.TrimSpace(strings.Join(texts, " "))
		if joined != "" {
			paragraphs = append(paragraphs, joined)
		}
		texts = nil
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local != "t" {
				continue
			}
			var value string
			if err := decoder.DecodeElement(&value, &element); err != nil {
				return nil, err
			}
			value = strings.TrimSpace(value)
			if value != "" {
				texts = append(texts, value)
			}
		case xml.EndElement:
			if element.Name.Local == "p" {
				flush()
			}
		}
	}
	flush()
	return paragraphs, nil
}

func readZipFile(files []*zip.File, name string) ([]byte, error) {
	for _, file := range files {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, os.ErrNotExist
}

func copyFileUnique(srcPath, dstDir string) (string, error) {
	dstPath, err := uniquePath(dstDir, filepath.Base(srcPath))
	if err != nil {
		return "", err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return dstPath, dst.Close()
}

func moveFileUnique(srcPath, dstDir string) (string, error) {
	dstPath, err := uniquePath(dstDir, filepath.Base(srcPath))
	if err != nil {
		return "", err
	}
	if err := os.Rename(srcPath, dstPath); err == nil {
		return dstPath, nil
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(srcPath); err != nil {
		return "", err
	}
	return dstPath, nil
}

func uniquePath(dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = "file"
	}
	candidate := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
	}
}

func displayTitleFromFilename(name string) string {
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	stem = strings.ReplaceAll(stem, "-", " ")
	stem = strings.ReplaceAll(stem, "_", " ")
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return "Imported document"
	}
	return stem
}

func chooseSourceDocumentTitle(explicitTitle, body, originalFilename string) string {
	if title := strings.TrimSpace(explicitTitle); title != "" {
		return title
	}
	if heading := stripMarkdownHeadingPrefix(firstMarkdownHeading(body)); heading != "" && !isGenericSourceDocumentTitle(heading) {
		return heading
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = stripMarkdownHeadingPrefix(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "|") || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "_") {
			continue
		}
		if isGenericSourceDocumentTitle(line) {
			continue
		}
		return truncateRunes(line, 80)
	}
	return displayTitleFromFilename(originalFilename)
}

func stripMarkdownHeadingPrefix(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, "#")
	return strings.TrimSpace(value)
}

func isGenericSourceDocumentTitle(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	return genericSourceDocumentTitlePattern.MatchString(value)
}

func naturalCompare(a, b string) int {
	ai := naturalParts(a)
	bi := naturalParts(b)
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if ai[i] == bi[i] {
			continue
		}
		aNum, aErr := strconv.Atoi(ai[i])
		bNum, bErr := strconv.Atoi(bi[i])
		if aErr == nil && bErr == nil {
			if aNum != bNum {
				return aNum - bNum
			}
			continue
		}
		return strings.Compare(ai[i], bi[i])
	}
	return len(ai) - len(bi)
}

func naturalParts(value string) []string {
	value = strings.ToLower(value)
	if value == "" {
		return nil
	}
	parts := []string{}
	var current strings.Builder
	lastDigit := false
	for i, r := range value {
		isDigit := r >= '0' && r <= '9'
		if i == 0 {
			lastDigit = isDigit
		}
		if i > 0 && isDigit != lastDigit {
			parts = append(parts, current.String())
			current.Reset()
		}
		current.WriteRune(r)
		lastDigit = isDigit
	}
	parts = append(parts, current.String())
	return parts
}

func parseRowIndex(ref string) int {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return -1
	}
	for i, r := range ref {
		if r >= '0' && r <= '9' {
			value, err := strconv.Atoi(ref[i:])
			if err != nil {
				return -1
			}
			return value - 1
		}
	}
	return -1
}

func parseCellColumn(ref string) int {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return -1
	}
	col := 0
	found := false
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			col = col*26 + int(r-'A'+1)
			found = true
			continue
		}
		if r >= 'a' && r <= 'z' {
			col = col*26 + int(r-'a'+1)
			found = true
			continue
		}
		break
	}
	if !found {
		return -1
	}
	return col - 1
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, attr := range attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func errorsIsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
