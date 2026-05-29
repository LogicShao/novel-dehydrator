package exporter

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const chapterMetaDelimiter = "---CHAPTER_META---"

type bookRow struct {
	Title  string
	Author string
}

type chapterRow struct {
	ID             int64
	Title          string
	Seq            int
	RawPath        string
	DehydratedPath *string
	Status         string
}

func ExportTXT(ctx context.Context, pool *pgxpool.Pool, bookID int64, dataDir string) ([]byte, error) {
	book, chapters, err := loadExportData(ctx, pool, bookID)
	if err != nil {
		return nil, err
	}
	return buildTXT(book.Title, chapters, dataDir)
}

func ExportEPUB(ctx context.Context, pool *pgxpool.Pool, bookID int64, dataDir string) ([]byte, error) {
	book, chapters, err := loadExportData(ctx, pool, bookID)
	if err != nil {
		return nil, err
	}
	return buildEPUB(book.Title, book.Author, chapters, dataDir)
}

func StripMeta(text string) string {
	idx := strings.Index(text, chapterMetaDelimiter)
	if idx == -1 {
		return text
	}
	return strings.TrimRight(text[:idx], "\r\n \t")
}

func loadExportData(ctx context.Context, pool *pgxpool.Pool, bookID int64) (bookRow, []chapterRow, error) {
	if pool == nil {
		return bookRow{}, nil, fmt.Errorf("exporter: nil pool")
	}

	var book bookRow
	err := pool.QueryRow(ctx, "SELECT title, author FROM books WHERE id=$1", bookID).Scan(&book.Title, &book.Author)
	if err != nil {
		return bookRow{}, nil, fmt.Errorf("exporter: query book: %w", err)
	}

	rows, err := pool.Query(ctx, `SELECT id, title, seq, raw_path, dehydrated_path, dehydrate_status
		FROM chapters
		WHERE book_id=$1 AND dehydrate_status='done'
		ORDER BY seq`, bookID)
	if err != nil {
		return bookRow{}, nil, fmt.Errorf("exporter: query chapters: %w", err)
	}
	defer rows.Close()

	chapters := make([]chapterRow, 0)
	for rows.Next() {
		var chapter chapterRow
		if err := rows.Scan(&chapter.ID, &chapter.Title, &chapter.Seq, &chapter.RawPath, &chapter.DehydratedPath, &chapter.Status); err != nil {
			return bookRow{}, nil, fmt.Errorf("exporter: scan chapter: %w", err)
		}
		chapters = append(chapters, chapter)
	}
	if err := rows.Err(); err != nil {
		return bookRow{}, nil, fmt.Errorf("exporter: iterate chapters: %w", err)
	}

	return book, chapters, nil
}

func buildTXT(bookTitle string, chapters []chapterRow, dataDir string) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("《")
	builder.WriteString(bookTitle)
	builder.WriteString("》脱水版\n\n")

	for _, chapter := range chapters {
		text := "（章节内容缺失）"
		if path := preferredPath(chapter); path != "" {
			content, err := os.ReadFile(resolvePath(dataDir, path))
			if err == nil {
				text = StripMeta(string(content))
			}
		}

		builder.WriteString("\n")
		builder.WriteString(chapter.Title)
		builder.WriteString("\n\n")
		builder.WriteString(text)
		builder.WriteString("\n")
	}

	return []byte(builder.String()), nil
}

func buildEPUB(bookTitle, author string, chapters []chapterRow, dataDir string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	if err := writeStoredFile(zw, "mimetype", []byte("application/epub+zip")); err != nil {
		return nil, err
	}
	if err := writeDeflatedFile(zw, "META-INF/container.xml", []byte(containerXML)); err != nil {
		return nil, err
	}

	manifestItems := []string{
		`<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>`,
		`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`,
	}
	spineItems := []string{}
	navLinks := make([]string, 0, len(chapters))
	ncxPoints := make([]string, 0, len(chapters))

	for i, chapter := range chapters {
		fileName := fmt.Sprintf("chapter_%d.xhtml", chapter.ID)
		itemID := fmt.Sprintf("chapter_%d", chapter.ID)
		text := "（章节内容缺失）"
		if path := preferredPath(chapter); path != "" {
			content, err := os.ReadFile(resolvePath(dataDir, path))
			if err == nil {
				text = StripMeta(string(content))
			}
		}

		chapterHTML := renderChapterHTML(chapter.Title, text)
		if err := writeDeflatedFile(zw, "OEBPS/"+fileName, []byte(chapterHTML)); err != nil {
			return nil, err
		}

		manifestItems = append(manifestItems, fmt.Sprintf(`<item id="%s" href="%s" media-type="application/xhtml+xml"/>`, itemID, fileName))
		spineItems = append(spineItems, fmt.Sprintf(`<itemref idref="%s"/>`, itemID))
		navLinks = append(navLinks, fmt.Sprintf(`<li><a href="%s">%s</a></li>`, fileName, escapeXML(chapter.Title)))
		ncxPoints = append(ncxPoints, fmt.Sprintf(`<navPoint id="%s" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="%s"/></navPoint>`, itemID, i+1, escapeXML(chapter.Title), fileName))
	}

	if err := writeDeflatedFile(zw, "OEBPS/nav.xhtml", []byte(renderNavHTML(bookTitle, navLinks))); err != nil {
		return nil, err
	}
	if err := writeDeflatedFile(zw, "OEBPS/toc.ncx", []byte(renderNCX(bookTitle, ncxPoints))); err != nil {
		return nil, err
	}
	if err := writeDeflatedFile(zw, "OEBPS/content.opf", []byte(renderOPF(bookTitle, author, manifestItems, spineItems))); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("exporter: close epub zip: %w", err)
	}

	return buf.Bytes(), nil
}

func preferredPath(chapter chapterRow) string {
	if chapter.DehydratedPath != nil && *chapter.DehydratedPath != "" {
		return *chapter.DehydratedPath
	}
	return chapter.RawPath
}

func resolvePath(dataDir, path string) string {
	if path == "" || filepath.IsAbs(path) || dataDir == "" {
		return path
	}
	return filepath.Join(dataDir, path)
}

func renderChapterHTML(title, text string) string {
	paragraphs := make([]string, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paragraphs = append(paragraphs, "<p>"+html.EscapeString(line)+"</p>")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="zh-CN">
<head><title>%s</title></head>
<body><h2>%s</h2>%s</body>
</html>`, escapeXML(title), escapeXML(title), strings.Join(paragraphs, ""))
}

func renderNavHTML(bookTitle string, links []string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="zh-CN">
<head><title>%s</title></head>
<body><nav epub:type="toc" id="toc"><h1>%s</h1><ol>%s</ol></nav></body>
</html>`, escapeXML(bookTitle), escapeXML(bookTitle), strings.Join(links, ""))
}

func renderNCX(bookTitle string, points []string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head></head>
<docTitle><text>%s</text></docTitle>
<navMap>%s</navMap>
</ncx>`, escapeXML(bookTitle), strings.Join(points, ""))
}

func renderOPF(bookTitle, author string, manifestItems, spineItems []string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid" version="3.0">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="bookid">novel-dehydrator-%s</dc:identifier>
<dc:title>%s 脱水版</dc:title>
<dc:language>zh</dc:language>
<dc:creator>%s</dc:creator>
</metadata>
<manifest>%s</manifest>
<spine>%s</spine>
</package>`, escapeXML(bookTitle), escapeXML(bookTitle), escapeXML(author), strings.Join(manifestItems, ""), strings.Join(spineItems, ""))
}

func escapeXML(s string) string {
	return html.EscapeString(s)
}

func writeStoredFile(zw *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("exporter: create zip entry %s: %w", name, err)
	}
	if _, err := writer.Write(content); err != nil {
		return fmt.Errorf("exporter: write zip entry %s: %w", name, err)
	}
	return nil
}

func writeDeflatedFile(zw *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("exporter: create zip entry %s: %w", name, err)
	}
	if _, err := writer.Write(content); err != nil {
		return fmt.Errorf("exporter: write zip entry %s: %w", name, err)
	}
	return nil
}

const containerXML = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
